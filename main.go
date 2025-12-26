package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"sync"
)

type LoginRequest struct {
	ID       string `json:"id"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Message string `json:"message"`
}

type UserResponse struct {
	LoggedIn bool   `json:"loggedIn"`
	ID       string `json:"id,omitempty"`
}

// 간단한 세션 저장소 (실무에서는 Redis, DB 등 사용)
var (
	sessions = make(map[string]string) // sessionID -> userID
	mu       sync.RWMutex
)

func main() {
	// 정적 파일 서빙
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))

	// "/" → main.html
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.ServeFile(w, r, "./static/main.html")
			return
		}
		http.NotFound(w, r)
	})

	// /login.html
	http.HandleFunc("/login.html", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/login.html")
	})

	// /main.html
	http.HandleFunc("/main.html", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/main.html")
	})

	// API 라우트
	http.HandleFunc("/api/login", loginHandler)
	http.HandleFunc("/api/logout", logoutHandler)
	http.HandleFunc("/api/user", userHandler)

	log.Println("Server started at :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(LoginResponse{
			Message: "Method Not Allowed",
		})
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(LoginResponse{
			Message: "Bad Request",
		})
		return
	}

	// ⚠️ 테스트용 계정 (실무에선 DB)
	if req.ID == "admin" && req.Password == "1234" {
		// 세션 생성
		sessionID, err := generateSessionID()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(LoginResponse{
				Message: "Session creation failed",
			})
			return
		}

		mu.Lock()
		sessions[sessionID] = req.ID
		mu.Unlock()

		// 쿠키 설정
		http.SetCookie(w, &http.Cookie{
			Name:     "session_id",
			Value:    sessionID,
			Path:     "/",
			HttpOnly: true,
		})

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(LoginResponse{
			Message: "Login Success",
		})
		return
	}

	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(LoginResponse{
		Message: "Invalid ID or Password",
	})
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// 쿠키에서 세션 ID 가져오기
	cookie, err := r.Cookie("session_id")
	if err == nil {
		mu.Lock()
		delete(sessions, cookie.Value)
		mu.Unlock()
	}

	// 쿠키 삭제
	http.SetCookie(w, &http.Cookie{
		Name:   "session_id",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(LoginResponse{
		Message: "Logout Success",
	})
}

func userHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// 쿠키에서 세션 ID 가져오기
	cookie, err := r.Cookie("session_id")
	if err != nil {
		json.NewEncoder(w).Encode(UserResponse{
			LoggedIn: false,
		})
		return
	}

	// 세션 확인
	mu.RLock()
	userID, exists := sessions[cookie.Value]
	mu.RUnlock()

	if !exists {
		json.NewEncoder(w).Encode(UserResponse{
			LoggedIn: false,
		})
		return
	}

	json.NewEncoder(w).Encode(UserResponse{
		LoggedIn: true,
		ID:       userID,
	})
}

// 안전한 세션 ID 생성 (crypto/rand 사용)
func generateSessionID() (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
