package server

import (
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	jwtKeyFilename  = "jwt.key"
	authPassKey     = "auth.password_hash"
	authUsernameKey = "auth.username"
	jwtTTL          = 7 * 24 * time.Hour
	keyChars        = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	//The lower limit is consistent with the front-end verification on the setup page - if the verification is only placed on the front-end, it means it is not placed. Just type API directly.
	//Bypass. The upper limit is bcrypt's hard limit: more than 72 bytes GenerateFromPassword returns
	//ErrPasswordTooLong, it is better to block it in advance than to let the user receive an unclear "Password encryption failed" message.
	minPasswordRunes = 8
	maxPasswordBytes = 72
)

//errDataSourceUnavailable is the unified reply when password-related read operations fail. These handlers must not
//Treat "cannot read" as "not set": authInit used to let the unauthenticated request be overwritten when the database reported an error.
//Remove the existing administrator password.
const errDataSourceUnavailable = "Data source is temporarily unavailable; try again later"

//validatePassword returns an empty string to indicate passing, otherwise returns a Chinese reason that can be directly displayed to the user.
func validatePassword(pw string) string {
	if utf8.RuneCountInString(pw) < minPasswordRunes {
		return fmt.Sprintf("Password must be at least %d characters", minPasswordRunes)
	}
	if len(pw) > maxPasswordBytes {
		return fmt.Sprintf("Password cannot exceed %d bytes", maxPasswordBytes)
	}
	return ""
}

// loadOrCreateJWTKey reads the 32-byte signing key from keyDir/jwt.key. keyDir is
// the project base dir (next to the executable), NOT the browsable workspace root
// (dataDir) — the signing key must never be listable/downloadable via the file
// manager. Legacy installs kept it at dataDir/jwt.key; if present there and not yet
// at the new location, it is migrated (key preserved, so sessions stay valid) and
// the old file removed so it disappears from the workspace. On first run a random
// key is generated and persisted.
func loadOrCreateJWTKey(keyDir, dataDir string) ([]byte, error) {
	path := filepath.Join(keyDir, jwtKeyFilename)
	// one-time migration out of the old in-workspace location.
	if legacy := filepath.Join(dataDir, jwtKeyFilename); legacy != path {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if data, rerr := os.ReadFile(legacy); rerr == nil {
				if werr := os.WriteFile(path, data, 0o600); werr == nil {
					_ = os.Remove(legacy)
					log.Printf("[auth] Migrated JWT key from %s to %s (moved outside the browsable workspace)", legacy, path)
				}
			}
		}
	}
	if data, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(data))) >= 32 {
		return []byte(strings.TrimSpace(string(data))), nil
	}
	buf := make([]byte, 32)
	for i := range buf {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(keyChars))))
		if err != nil {
			return nil, fmt.Errorf("generate jwt key: %w", err)
		}
		buf[i] = keyChars[n.Int64()]
	}
	if err := os.WriteFile(path, buf, 0600); err != nil {
		return nil, fmt.Errorf("write jwt key: %w", err)
	}
	log.Printf("[auth] Wrote new JWT key to %s", path)
	return buf, nil
}

// signJWT issues a 7-day HS256 token. The default preserves existing callers.
func signJWT(key []byte, usernames ...string) (string, error) {
	username := "ARTEX"
	if len(usernames) > 0 {
		username = usernames[0]
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   username,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(jwtTTL)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}).SignedString(key)
}

// verifyJWT returns true when tokenStr is a valid, non-expired HS256 token.
func verifyJWT(tokenStr string, key []byte) bool {
	t, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return key, nil
	})
	return err == nil && t.Valid
}

// extractToken accepts credentials only in the Authorization header.
func extractToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}

// requireAuth wraps h with JWT validation.
// /api/auth/* and /api/health are exempt.
func (s *Server) requireAuth(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if strings.HasPrefix(p, "/api/auth/") || p == "/api/health" {
			h.ServeHTTP(w, r)
			return
		}
		tok := extractToken(r)
		if tok == "" {
			writeErr(w, 401, "Unauthorized")
			return
		}
		if !verifyJWT(tok, s.jwtKey) {
			writeErr(w, 401, "Token is invalid or expired")
			return
		}
		h.ServeHTTP(w, r)
	})
}

// GET /api/auth/status — reports whether the admin password has been initialised.
//Reading failure must return 503 instead of initialized:false: the front end will return the user when initialized:false
//Send it to /setup to set the password (login/page.tsx). Packaging the database failure into 200 is equivalent to sending the user to
//Overwrite the existing password on the road.
func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	hash, _, err := pg.GetSetting(authPassKey)
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	writeJSON(w, 200, map[string]any{"initialized": hash != ""})
}

// POST /api/auth/init — sets the password for the first time; rejected if already set.
func (s *Server) authInit(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	existing, _, err := pg.GetSetting(authPassKey)
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	if existing != "" {
		writeErr(w, 403, "Password is already set")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil || req.Password == "" {
		writeErr(w, 400, "Password cannot be empty")
		return
	}
	if msg := validatePassword(req.Password); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if msg := validateUsername(req.Username); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, 500, "Failed to hash password")
		return
	}
	// The database inserts the password only once and saves the username in the same transaction.
	inserted, err := pg.InitAccount(req.Username, string(hash))
	if err != nil {
		writeErr(w, 500, "Failed to save: "+err.Error())
		return
	}
	if !inserted {
		writeErr(w, 403, "Password is already set")
		return
	}
	tok, err := signJWT(s.jwtKey, req.Username)
	if err != nil {
		writeErr(w, 500, "Failed to generate token")
		return
	}
	writeJSON(w, 200, map[string]any{"token": tok})
}

// POST /api/auth/change-password — changes the admin username and/or password. Requires a valid
// token (this route is under /api/auth/* which requireAuth exempts, so the token
// is validated here) AND the current password.
func (s *Server) authChangePassword(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	if !verifyJWT(extractToken(r), s.jwtKey) {
		writeErr(w, 401, "Unauthorized")
		return
	}
	var req struct {
		Username    *string `json:"username"`
		OldPassword string  `json:"old_password"`
		NewPassword string  `json:"new_password"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "Invalid request format")
		return
	}
	if req.NewPassword == "" && req.Username == nil {
		writeErr(w, 400, "Enter a username or a new password")
		return
	}
	if msg := validatePassword(req.NewPassword); req.NewPassword != "" && msg != "" {
		writeErr(w, 400, msg)
		return
	}
	hash, ok, err := pg.GetSetting(authPassKey)
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	if !ok || hash == "" {
		writeErr(w, 403, "Password has not been initialized; set a password first")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.OldPassword)); err != nil {
		writeErr(w, 401, "Current password is incorrect")
		return
	}
	username, err := s.accountUsername()
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	if req.Username != nil {
		username = strings.TrimSpace(*req.Username)
		if msg := validateUsername(username); msg != "" {
			writeErr(w, 400, msg)
			return
		}
	}
	newHash := []byte(hash)
	if req.NewPassword != "" {
		newHash, err = bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			writeErr(w, 500, "Failed to hash password")
			return
		}
	}
	tok, err := signJWT(s.jwtKey, username)
	if err != nil {
		writeErr(w, 500, "Failed to generate token")
		return
	}
	updated, err := pg.UpdateAccount(username, hash, string(newHash))
	if err != nil {
		writeErr(w, 500, "Failed to save: "+err.Error())
		return
	}
	if !updated {
		writeErr(w, 409, "Account changed; enter the current password and try again")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "token": tok})
}

// POST /api/auth/login — validates username/password and returns a JWT.
func (s *Server) authLogin(w http.ResponseWriter, r *http.Request) {
	if !s.loginGate.allow(r.RemoteAddr, time.Now()) {
		w.Header().Set("Retry-After", "60")
		writeErr(w, http.StatusTooManyRequests, "Too many login attempts; try again later")
		return
	}
	pg := s.pg(w)
	if pg == nil {
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "Invalid request format")
		return
	}
	username, err := s.accountUsername()
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	if strings.TrimSpace(req.Username) != username {
		writeErr(w, 401, "Incorrect username or password")
		return
	}
	hash, ok, err := pg.GetSetting(authPassKey)
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	if !ok || hash == "" {
		writeErr(w, 403, "Password has not been initialized; set a password first")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		writeErr(w, 401, "Incorrect username or password")
		return
	}
	tok, err := signJWT(s.jwtKey, username)
	if err != nil {
		writeErr(w, 500, "Failed to generate token")
		return
	}
	writeJSON(w, 200, map[string]any{"token": tok})
}

func validateUsername(username string) string {
	if username == "" || utf8.RuneCountInString(username) > 64 {
		return "Username must be between 1 and 64 characters"
	}
	for _, ch := range username {
		if unicode.IsSpace(ch) || unicode.IsControl(ch) {
			return "Username cannot contain spaces or control characters"
		}
	}
	return ""
}

func (s *Server) accountUsername() (string, error) {
	username, ok, err := s.m.pg.GetSetting(authUsernameKey)
	if err != nil {
		return "", err
	}
	if !ok {
		return "ARTEX", nil
	}
	return username, nil
}

// authAccount exposes the saved username only to authenticated sessions.
func (s *Server) authAccount(w http.ResponseWriter, r *http.Request) {
	if !verifyJWT(extractToken(r), s.jwtKey) {
		writeErr(w, 401, "Unauthorized")
		return
	}
	if s.pg(w) == nil {
		return
	}
	username, err := s.accountUsername()
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	writeJSON(w, 200, map[string]any{"username": username})
}
