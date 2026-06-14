package turn

import httpbase "co-browsing-session-server/internal/interfaces/http"

// Input은 입력이 없음을 나타낸다 — 인증/파라미터 불필요 (FR-7, AC-6).
type Input struct{}

// Credentials는 TURN 중계 서버 접속 자격증명 페이로드다.
type Credentials struct {
	Username string   `json:"username" doc:"TURN 접속 사용자명. {만료_unix_timestamp}:cobrowsing 형식" example:"1716003600:cobrowsing"`
	Password string   `json:"password" doc:"base64(HMAC-SHA1(secret, username))로 계산된 임시 비밀번호"`
	TTL      int      `json:"ttl" doc:"자격증명 유효 기간(초). 발급 시점 기준" example:"3600"`
	URIs     []string `json:"uris" doc:"접속 대상 TURN 서버 주소 목록(하나 이상)"`
}

// Response는 공통 봉투 SuccessResponse[T]를 입는다 → JSON 상 {"data": {...}}.
type Response = httpbase.SuccessResponse[Credentials]
