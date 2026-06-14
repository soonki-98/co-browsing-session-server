package turn

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

const (
	ttlSeconds = 3600

	defaultSecret = "changeme"
	defaultURI    = "turn:localhost:3478"
)

// Handler는 TURN 자격증명 발급 어댑터다. stateless 계산만 하므로 의존이 없다.
type Handler struct{}

// NewHandler는 TURN 자격증명 핸들러를 만든다.
func NewHandler() *Handler {
	return &Handler{}
}

// Register는 huma API에 GET /turn-credentials 오퍼레이션을 등록한다.
func (h *Handler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "getTURNCredentials",
		Method:      http.MethodGet,
		Path:        "/turn-credentials",
		Summary:     "TURN 자격증명 발급",
		Description: "WebRTC 폴백용 TURN 중계 서버 단기 접속 자격증명을 발급합니다.",
		Tags:        []string{"turn"},
	}, h.GetCredentials)
}

// GetCredentials는 발급 시점 기준 TTL을 갖는 임시 TURN 자격증명을 stateless하게 계산한다.
// username = "{만료_unix_timestamp}:cobrowsing", password = base64(HMAC-SHA1(secret, username)).
func (h *Handler) GetCredentials(_ context.Context, _ *Input) (*Response, error) {
	expiry := time.Now().Unix() + ttlSeconds
	username := fmt.Sprintf("%d:cobrowsing", expiry)

	mac := hmac.New(sha1.New, []byte(turnSecret()))
	mac.Write([]byte(username))
	password := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	response := &Response{}
	response.Body.Data = Credentials{
		Username: username,
		Password: password,
		TTL:      ttlSeconds,
		URIs:     turnURIs(),
	}
	return response, nil
}

// turnSecret는 HMAC 서명 키를 환경변수에서 읽고, 미설정 시 개발용 기본값으로 폴백한다.
// 시크릿은 응답·로그 어디에도 노출하지 않는다.
func turnSecret() string {
	if s := os.Getenv("TURN_SECRET"); s != "" {
		return s
	}
	return defaultSecret
}

// turnURIs는 TURN_URIS를 쉼표로 분리·트림해 반환하며, 미설정/공백이면 기본 URI 하나를 보장한다.
func turnURIs() []string {
	raw := os.Getenv("TURN_URIS")
	if raw == "" {
		return []string{defaultURI}
	}
	parts := strings.Split(raw, ",")
	uris := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			uris = append(uris, trimmed)
		}
	}
	if len(uris) == 0 {
		return []string{defaultURI}
	}
	return uris
}
