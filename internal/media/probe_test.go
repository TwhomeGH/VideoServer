package media

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

const validProbe = `{"format":{"duration":"2.125","format_name":"mov,mp4"},"streams":[{"codec_type":"video","codec_name":"h264","width":320,"height":180,"avg_frame_rate":"24000/1001"},{"codec_type":"audio","codec_name":"aac"}]}`

func TestMain(m *testing.M) {
	if mode := os.Getenv("DVN_PROBE_TEST_HELPER"); mode != "" {
		if mode == "timeout" {
			time.Sleep(5 * time.Second)
			os.Exit(0)
		}
		args := strings.Join(os.Args[1:], " ")
		if !strings.Contains(args, "-protocol_whitelist file") || !strings.Contains(args, "-format_whitelist mov,matroska,webm,avi,m4v") {
			os.Exit(2)
		}
		fmt.Print(validProbe)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestProbeParsesVideoAndRejectsAudioCover(t *testing.T) {
	info, err := parse([]byte(validProbe))
	if err != nil || info.Width != 320 || info.Height != 180 || info.VideoCodec != "h264" || info.AudioCodec != "aac" || info.DurationSeconds != 2.125 || info.FrameRate < 23.9 {
		t.Fatalf("invalid info: %+v %v", info, err)
	}
	if _, err := parse([]byte(`{"streams":[{"codec_type":"video","codec_name":"mjpeg","width":100,"height":100,"disposition":{"attached_pic":1}}]}`)); err == nil {
		t.Fatal("audio cover accepted as video")
	}
	if _, err := parse([]byte("not json")); err == nil {
		t.Fatal("invalid JSON accepted")
	}
}

func TestProbeProcessAndTimeout(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DVN_PROBE_TEST_HELPER", "valid")
	if _, err := Probe(context.Background(), "local file.mp4", executable, 3*time.Second); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DVN_PROBE_TEST_HELPER", "timeout")
	if _, err := Probe(context.Background(), "file.mp4", executable, 100*time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected bounded timeout, got %v", err)
	}
	if _, err := Probe(context.Background(), "file.mp4", "missing-dvn-ffprobe-executable", time.Second); err == nil {
		t.Fatal("missing executable accepted")
	}
	buffer := &limitedBuffer{limit: 4}
	if _, err := buffer.Write([]byte("oversized output")); err == nil || buffer.Len() != 4 {
		t.Fatal("output limit was not enforced")
	}
}
