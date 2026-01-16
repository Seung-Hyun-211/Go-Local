package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"

	_ "github.com/go-sql-driver/mysql"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

// Config 구조체
type Config struct {
	ApiKey          string `json:"ApiKey"`
	ApplicationName string `json:"ApplicationName"`
}

var (
	appConfig      Config
	youtubeService *youtube.Service
)

func handleError(err error, message string) {
	if err != nil {
		log.Printf("%s: %v", message, err)
	}
}

func getExeDir() string {
	exe, err := os.Executable()
	if err != nil {
		cwd, _ := os.Getwd()
		return cwd
	}

	dir := filepath.Dir(exe)
	if strings.Contains(dir, "go-build") || strings.Contains(dir, "Temp") || strings.Contains(dir, "tmp") {
		cwd, _ := os.Getwd()
		return cwd
	}
	return dir
}

func sanitizeFilename(name string) string {
	reg := regexp.MustCompile(`[\\/:*?"<>|]`)
	sanitized := reg.ReplaceAllString(name, " ")
	return strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(sanitized, " "))
}

type VideoResult struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Duration int    `json:"duration"`
}

type JSONResponse struct {
	Success       bool                   `json:"success"`
	Error         string                 `json:"error,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	SearchResults []VideoResult          `json:"search_results,omitempty"`
	LocalPath     string                 `json:"local_path,omitempty"`
	DBStatus      string                 `json:"db_status,omitempty"`
}

func loadConfig() error {
	configPath := filepath.Join(getExeDir(), "config.json")
	file, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	return json.Unmarshal(file, &appConfig)
}

func initYouTubeService() error {
	ctx := context.Background()
	var err error
	youtubeService, err = youtube.NewService(ctx, option.WithAPIKey(appConfig.ApiKey))
	return err
}

func saveToDB(videoID, title, channelID string, duration int) error {
	dsn := "lee:rhqckd211@tcp(127.0.0.1:3306)/testdb?charset=utf8mb4&parseTime=true"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return err
	}

	query := `INSERT INTO youtube_videos (id, title, channel_id, duration) 
	          VALUES (?, ?, ?, ?) 
	          ON DUPLICATE KEY UPDATE title=?, channel_id=?, duration=?`
	_, err = db.Exec(query, videoID, title, channelID, duration, title, channelID, duration)
	return err
}

func getTargetPCMPath(channelTitle, title string) string {
	return filepath.Join(getExeDir(), "db", sanitizeFilename(channelTitle), sanitizeFilename(title)+".pcm")
}

func downloadAndConvert(videoURL, targetPath string) error {
	dirPath := filepath.Dir(targetPath)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return err
	}

	opusPath := strings.ReplaceAll(targetPath, ".pcm", ".opus")
	downloadCmd := exec.Command("yt-dlp",
		"-x",
		"--audio-format", "opus",
		"--audio-quality", "0",
		"--no-playlist",
		"--no-part",
		"--no-mtime",
		"-o", opusPath,
		videoURL,
	)

	if out, err := downloadCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("download failed: %v, output: %s", err, string(out))
	}
	defer os.Remove(opusPath)

	pcmData, err := convertOpusToPCM(opusPath)
	if err != nil {
		return err
	}

	return os.WriteFile(targetPath, pcmData, 0644)
}

func handleProcess(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	query := r.URL.Query().Get("q")
	urlInput := r.URL.Query().Get("url")

	// 디버그용 로그 출력
	if urlInput != "" {
		fmt.Printf("[DEBUG] 요청 수신 (URL): %s\n", urlInput)
	} else if query != "" {
		fmt.Printf("[DEBUG] 요청 수신 (검색어): %s\n", query)
	}

	var targetVideoID string
	var searchResults []VideoResult

	if urlInput != "" {
		// URL에서 ID 추출 (간단한 예시)
		if strings.Contains(urlInput, "v=") {
			targetVideoID = strings.Split(strings.Split(urlInput, "v=")[1], "&")[0]
		} else if strings.Contains(urlInput, "youtu.be/") {
			targetVideoID = strings.Split(strings.Split(urlInput, "youtu.be/")[1], "?")[0]
		} else {
			targetVideoID = urlInput // ID 자체가 들어왔을 경우
		}
	} else if query != "" {
		// YouTube API를 이용한 검색
		call := youtubeService.Search.List([]string{"id", "snippet"}).
			Q(query).
			MaxResults(5).
			Type("video")

		response, err := call.Do()
		if err != nil {
			json.NewEncoder(w).Encode(JSONResponse{Success: false, Error: "Search failed: " + err.Error()})
			return
		}

		for _, item := range response.Items {
			searchResults = append(searchResults, VideoResult{
				ID:    item.Id.VideoId,
				Title: item.Snippet.Title,
			})
		}

		if len(searchResults) > 0 {
			targetVideoID = searchResults[0].ID
		} else {
			json.NewEncoder(w).Encode(JSONResponse{Success: false, Error: "No results found"})
			return
		}
	} else {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Error: "Missing query or url parameter"})
		return
	}

	// 영여성 등 메타데이터 상세 정보 가져오기
	videoCall := youtubeService.Videos.List([]string{"snippet", "contentDetails", "statistics"}).Id(targetVideoID)
	videoResponse, err := videoCall.Do()
	if err != nil || len(videoResponse.Items) == 0 {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Error: "Failed to get video details"})
		return
	}

	video := videoResponse.Items[0]
	title := video.Snippet.Title
	channelID := video.Snippet.ChannelId
	channelTitle := video.Snippet.ChannelTitle

	// ISO 8601 duration 파싱 (생략하거나 간단히 처리하려면 yt-dlp 활용 가능하지만 여기선 DB용으로 저장 필요)
	// 여기서는 단순히 저장 구조를 보여주기 위해 duration을 0으로 처리하거나 yt-dlp 출력에서 가져올 수 있음
	// 일단 0으로 두고 DB 저장 로직 수행
	duration := 0

	targetPath := getTargetPCMPath(channelTitle, title)

	var dbStatus string
	if err := saveToDB(targetVideoID, title, channelTitle, duration); err != nil {
		dbStatus = "DB Save Failed: " + err.Error()
	} else {
		dbStatus = "Success"
	}

	response := JSONResponse{
		Success: true,
		Metadata: map[string]interface{}{
			"id":           targetVideoID,
			"title":        title,
			"channelId":    channelID,
			"channelTitle": channelTitle,
		},
		SearchResults: searchResults,
		LocalPath:     targetPath,
		DBStatus:      dbStatus,
	}

	// 파일 존재 여부 확인 및 다운로드
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		fmt.Printf("[DEBUG] 파일 없음, 다운로드 시작: %s\n", targetPath)
		videoURL := "https://www.youtube.com/watch?v=" + targetVideoID
		if err := downloadAndConvert(videoURL, targetPath); err != nil {
			fmt.Printf("[DEBUG] 다운로드/변환 실패: %v\n", err)
			response.Success = false
			response.Error = "Download/Convert failed: " + err.Error()
		} else {
			fmt.Printf("[DEBUG] 다운로드/변환 완료: %s\n", targetPath)
		}
	} else {
		fmt.Printf("[DEBUG] 파일 이미 존재함: %s\n", targetPath)
	}

	json.NewEncoder(w).Encode(response)
}

func main() {
	if runtime.GOOS == "windows" {
		// Windows 콘솔 입력/출력을 UTF-8(65001)로 설정
		kernel32 := syscall.NewLazyDLL("kernel32.dll")
		kernel32.NewProc("SetConsoleCP").Call(uintptr(65001))
		kernel32.NewProc("SetConsoleOutputCP").Call(uintptr(65001))
	}

	if err := loadConfig(); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := initYouTubeService(); err != nil {
		log.Fatalf("Failed to init YouTube service: %v", err)
	}

	http.HandleFunc("/process", handleProcess)

	port := ":8080"
	fmt.Printf("Server starting on http://localhost%s\n", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
