# 07. TURN Credentials Spec

## Overview

WebRTC P2P 연결 실패 시 폴백으로 사용하는 TURN 서버의 임시 자격증명을 발급한다.  
전체 8단계 구현 중 **7번째** — 다른 컴포넌트와 독립적. Session Store나 WebSocket Hub에 의존하지 않는다.

---

## Implementation Order

```
[1] Session Store ✓
[2] Serial Number Update ✓
[3] WebSocket Hub ✓
[4] WebSocket Handler ✓
[5] Signaling Protocol ✓
[6] Control Event Relay ✓
[7] TURN Credentials  ← 지금 여기
[8] CORS Middleware
```

- **선행 의존성:** 없음 (독립 구현 가능)
- **후행 의존성:** 없음

---

## Dependencies

```go
import (
    "crypto/hmac"
    "crypto/sha1"
    "encoding/base64"
    "fmt"
    "net/http"
    "os"
    "time"
    "github.com/gin-gonic/gin"
)
```

신규 외부 패키지 없음.

---

## Data Structures

```go
// TURN 자격증명 응답 구조체
type TURNCredentials struct {
    Username string   `json:"username"`
    Password string   `json:"password"`
    TTL      int      `json:"ttl"`      // 초 단위 (3600 = 1시간)
    URIs     []string `json:"uris"`     // TURN 서버 주소 목록
}
```

---

## Interfaces / Contracts

### HTTP Endpoint

```
GET /turn-credentials

Response 200 OK:
{
  "username": "1716003600:user",
  "password": "base64encodedHMACsha1",
  "ttl": 3600,
  "uris": [
    "turn:turn.example.com:3478?transport=udp",
    "turn:turn.example.com:3478?transport=tcp",
    "turns:turn.example.com:5349?transport=tcp"
  ]
}
```

### 핸들러 생성자

```go
func NewTURNCredentialsHandler() gin.HandlerFunc
func RegisterTURNCredentialsRoutes(router *gin.Engine)
```

---

## Behavior

### HMAC-SHA1 임시 자격증명 생성 (Coturn 호환 방식)

```
1. 만료 시각 계산:
   expiry = time.Now().Unix() + 3600

2. username 구성:
   username = fmt.Sprintf("%d:cobrowsing", expiry)

3. HMAC-SHA1 서명:
   secret = os.Getenv("TURN_SECRET")  // 환경변수에서 로드
   mac = hmac.New(sha1.New, []byte(secret))
   mac.Write([]byte(username))
   password = base64.StdEncoding.EncodeToString(mac.Sum(nil))

4. 응답 조립:
   return TURNCredentials{
       Username: username,
       Password: password,
       TTL:      3600,
       URIs:     loadTURNURIs(),  // 환경변수 TURN_URIS 또는 기본값
   }
```

### 환경변수

| 변수명 | 기본값 | 설명 |
|--------|--------|------|
| `TURN_SECRET` | `"changeme"` | HMAC 서명 키 (프로덕션에서 반드시 변경) |
| `TURN_URIS` | `"turn:localhost:3478"` | 쉼표 구분 TURN 서버 URI 목록 |

기본값은 MVP 로컬 개발용이며, 프로덕션 배포 전 환경변수로 재설정 필요.

### TURN 서버 미설정 시 동작

`TURN_SECRET`이 기본값이면 응답은 반환하되, 실제 TURN 서버가 없으므로 클라이언트의 릴레이 연결은 실패한다. 이는 MVP에서 허용되는 동작이다 (P2P가 성공하면 TURN은 사용되지 않음).

---

## File Locations

| 작업 | 파일 |
|------|------|
| 신규 생성 | `internal/handler/turn_credentials.go` |
| 수정 | `main.go` (라우트 등록 추가) |

---

## Acceptance Criteria

- [ ] `GET /turn-credentials` → 200 OK, username/password/ttl/uris 필드 모두 포함
- [ ] username 형식: `{unix_timestamp}:cobrowsing`
- [ ] TTL = 3600
- [ ] TURN_SECRET 환경변수 설정 시 해당 키로 HMAC 서명
- [ ] 연속 두 번 호출 시 username(=timestamp 기반)이 다를 수 있음 (1초 이상 간격이면)
- [ ] 인증 불필요 (MVP)
