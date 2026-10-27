package protocol_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/TwhomeGH/VideoServer/internal/diagnostics"
	"github.com/TwhomeGH/VideoServer/internal/library"
	"github.com/TwhomeGH/VideoServer/internal/media"
	dvnRuntime "github.com/TwhomeGH/VideoServer/internal/runtime"
	"github.com/TwhomeGH/VideoServer/pkg/buildinfo"
	"github.com/TwhomeGH/VideoServer/pkg/protocol"
)

func specification(t *testing.T, service string) map[string]any {
	t.Helper()
	value, err := protocol.Document(service)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(value, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestProtocolRejectsBeforeSideEffects(t *testing.T) {
	for _, test := range []struct {
		values []string
		status int
	}{
		{nil, 202}, {[]string{"1"}, 202}, {[]string{"2"}, 409}, {[]string{"999"}, 409},
		{[]string{""}, 400}, {[]string{"0"}, 400}, {[]string{"-1"}, 400}, {[]string{"01"}, 400},
		{[]string{"+1"}, 400}, {[]string{"1,2"}, 400}, {[]string{"1", "1"}, 400}, {[]string{"9999999999999999999999999999999"}, 400},
	} {
		t.Run(strings.Join(test.values, ","), func(t *testing.T) {
			called := false
			handler := protocol.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(202) }))
			request := httptest.NewRequest("POST", "/api/v1/system/restart", nil)
			if test.values != nil {
				for _, value := range test.values {
					request.Header.Add(protocol.Header, value)
				}
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status || called != (test.status == 202) || response.Header().Get(protocol.Header) != strconv.Itoa(protocol.Current) {
				t.Fatalf("status=%d called=%v body=%s", response.Code, called, response.Body.String())
			}
			if test.status != 202 {
				var body struct {
					Error, Code       string
					ProtocolVersion   int
					SupportedVersions []int
				}
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if body.Error == "" || body.Code == "" || body.ProtocolVersion != protocol.Current || !reflect.DeepEqual(body.SupportedVersions, []int{protocol.Current}) {
					t.Fatal("missing negotiation details")
				}
			}
		})
	}
	if !protocol.Compatible(1) || protocol.Compatible(0) || protocol.Compatible(2) {
		t.Fatal("incompatible peer accepted")
	}
}

func TestProtocolInfoAndNegotiation(t *testing.T) {
	mux := http.NewServeMux()
	protocol.Register(mux, "video-node")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/protocol", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("protocol status = %d", response.Code)
	}
	var info struct {
		Service            string           `json:"service"`
		ProtocolVersion    int              `json:"protocolVersion"`
		MinProtocolVersion int              `json:"minProtocolVersion"`
		MaxProtocolVersion int              `json:"maxProtocolVersion"`
		SupportedVersions  []int            `json:"supportedVersions"`
		Capabilities       []string         `json:"capabilities"`
		CapabilityVersions map[string][]int `json:"capabilityVersions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.Service != "video-node" || info.ProtocolVersion != protocol.Current || info.MinProtocolVersion != protocol.MinSupported || info.MaxProtocolVersion != protocol.MaxSupported ||
		!reflect.DeepEqual(info.SupportedVersions, protocol.SupportedVersions()) || !reflect.DeepEqual(info.Capabilities, protocol.Capabilities("video-node")) ||
		!reflect.DeepEqual(info.CapabilityVersions["library.query"], []int{1}) {
		t.Fatalf("unexpected protocol info: %+v", info)
	}
	if version, ok := protocol.Negotiate(1, 1); !ok || version != 1 {
		t.Fatal("v1 negotiation failed")
	}
	if version, ok := protocol.Negotiate(1, 2); !ok || version != 1 {
		t.Fatal("range negotiation failed")
	}
	for _, pair := range [][2]int{{0, 1}, {2, 3}, {3, 1}} {
		if _, ok := protocol.Negotiate(pair[0], pair[1]); ok {
			t.Fatalf("invalid negotiation accepted: %v", pair)
		}
	}
}

func TestServiceSpecificationsAndReferences(t *testing.T) {
	if len(protocol.Capabilities("unknown")) != 0 {
		t.Fatal("unknown service advertised capabilities")
	}
	if _, err := protocol.Document("unknown"); err == nil {
		t.Fatal("unknown service received specification")
	}
	for _, service := range []string{"video-node", "central", "transcode-worker"} {
		spec := specification(t, service)
		if spec["openapi"] != "3.1.1" || spec["x-dvn-protocol-version"] != float64(protocol.Current) {
			t.Fatal("spec version drift")
		}
		paths := spec["paths"].(map[string]any)
		usedTags := map[string]bool{}
		_, hasLibrary := paths["/api/v1/library"]
		if hasLibrary != (service == "video-node") {
			t.Fatal("service advertises unsupported API")
		}
		var walk func(any)
		walk = func(value any) {
			switch value := value.(type) {
			case map[string]any:
				if _, operation := value["operationId"]; operation {
					if tags, ok := value["tags"].([]any); ok {
						for _, tag := range tags {
							usedTags[tag.(string)] = true
						}
					}
				}
				if reference, ok := value["$ref"].(string); ok {
					if !strings.HasPrefix(reference, "#/") {
						t.Fatalf("external reference: %s", reference)
					}
					var target any = spec
					for _, key := range strings.Split(strings.TrimPrefix(reference, "#/"), "/") {
						object, ok := target.(map[string]any)
						if !ok {
							t.Fatalf("unresolved ref: %s", reference)
						}
						target, ok = object[key]
						if !ok {
							t.Fatalf("unresolved ref: %s", reference)
						}
					}
				}
				for _, child := range value {
					walk(child)
				}
			case []any:
				for _, child := range value {
					walk(child)
				}
			}
		}
		walk(spec)
		for _, tag := range spec["tags"].([]any) {
			name := tag.(map[string]any)["name"].(string)
			if !usedTags[name] {
				t.Errorf("%s advertises empty API group %s", service, name)
			}
		}
	}
}

func TestOpenAPICoversRegisteredRoutes(t *testing.T) {
	paths := specification(t, "video-node")["paths"].(map[string]any)
	root := filepath.Join("..", "..")
	for _, directory := range []string{"internal", "pkg/protocol"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok || len(call.Args) == 0 {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || (selector.Sel.Name != "HandleFunc" && selector.Sel.Name != "Handle") {
					return true
				}
				literal, ok := call.Args[0].(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return true
				}
				pattern, err := strconv.Unquote(literal.Value)
				if err != nil {
					t.Error(err)
					return true
				}
				parts := strings.SplitN(pattern, " ", 2)
				if len(parts) != 2 {
					return true
				}
				item, ok := paths[parts[1]].(map[string]any)
				if !ok || item[strings.ToLower(parts[0])] == nil {
					t.Errorf("undocumented route %q in %s", pattern, path)
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// jsonFields 依 encoding/json 的匿名欄位展開規則收集本專案回應型別。
func jsonFields(value reflect.Type) map[string]bool {
	fields := map[string]bool{}
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		if field.PkgPath != "" {
			continue
		}
		if field.Anonymous {
			for name, optional := range jsonFields(field.Type) {
				fields[name] = optional
			}
			continue
		}
		tag := strings.Split(field.Tag.Get("json"), ",")
		if tag[0] == "-" {
			continue
		}
		name := tag[0]
		if name == "" {
			name = field.Name
		}
		fields[name] = strings.Contains(field.Tag.Get("json"), ",omitempty")
	}
	return fields
}

func TestOpenAPIResponseFieldsMatchGoTypes(t *testing.T) {
	schemas := specification(t, "video-node")["components"].(map[string]any)["schemas"].(map[string]any)
	for name, value := range map[string]any{"BuildIdentity": buildinfo.Identity{}, "Config": dvnRuntime.Config{}, "Network": dvnRuntime.NetworkRuntime{}, "Video": library.Video{}, "Pool": library.Pool{}, "Media": media.Info{}, "ImportJob": library.ImportJob{}, "ScanStatus": library.ScanStatus{}, "ScanResult": library.ScanResult{}, "PoolScanStatus": library.PoolScanStatus{}, "BackfillStatus": library.BackfillStatus{}, "Monitor": diagnostics.Snapshot{}, "WorkStatus": library.WorkStatus{}} {
		schema := schemas[name].(map[string]any)
		properties := schema["properties"].(map[string]any)
		required := map[string]bool{}
		for _, key := range schema["required"].([]any) {
			required[key.(string)] = true
		}
		fields := jsonFields(reflect.TypeOf(value))
		if len(fields) != len(properties) {
			t.Errorf("%s fields differ: Go %d, spec %d", name, len(fields), len(properties))
		}
		for key, optional := range fields {
			if properties[key] == nil || required[key] == optional {
				t.Errorf("%s.%s missing or incorrect required status", name, key)
			}
		}
	}
}
