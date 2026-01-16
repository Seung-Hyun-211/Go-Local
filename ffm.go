package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
)

func getFFmpegPath() string {
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("ffmpeg"); err == nil {
			return "ffmpeg"
		}
		return `C:\ffmpeg\bin\ffmpeg.exe`
	}
	return "ffmpeg"
}

func convertOpusToPCM(opusPath string) ([]byte, error) {
	ffmpegPath := getFFmpegPath()

	args := []string{
		"-loglevel", "error",
		"-hide_banner",
		"-i", opusPath,

		"-vn", // 영상 제거
		"-ar", "48000",
		"-ac", "2",
		"-channel_layout", "stereo",
		"-acodec", "pcm_s16le",

		"-f", "s16le",
		"pipe:1",
	}

	cmd := exec.Command(ffmpegPath, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// stderr 비동기 소비 (중요)
	go func() {
		io.Copy(io.Discard, stderr)
	}()

	// PCM 데이터 읽기
	pcmData, err := io.ReadAll(stdout)
	if err != nil {
		return nil, err
	}

	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	return pcmData, nil
}

func main_old() {
	opusFile := "test.opus"

	pcm, err := convertOpusToPCM(opusFile)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Printf("PCM size: %d bytes\n", len(pcm))

	// 예시: 파일로 저장
	os.WriteFile("output.pcm", pcm, 0644)
}
