# TURN 자격증명 발급 — 기술 설계도

> 기획: [docs/specs/화면공유/turn_자격증명_발급.md](../../specs/화면공유/turn_자격증명_발급.md)
> 기존 기술 스펙(flat): [docs/specs/07-turn-credentials.md](../../specs/07-turn-credentials.md)

## Overview

WebRTC P2P(직접 연결) 실패 시 폴백으로 쓰이는 TURN 중계 서버의 **단기 접속 자격증명**을 발급하는 HTTP 엔드포인트다. Coturn 호환 방식(HMAC-SHA1 + base64)으로 매 요청마다 그 시점 기준 TTL을 갖는 임시 username/password를 생성하고, 접속 대상 TURN 서버 주소 목록과 함께 반환한다.

이 기능은 화면 공유의 다른 단계(세션 생성, WebSocket 시그널링, 제어 이벤트 중계)와 **완전히 독립적**이다. Domain Store(RoomSession/Invitation 저장소)나 WebSocket Hub에 의존하지 않으며, 진행 중인 세션 상태·실시간 연결과 무관하게 동작한다(NFR-4). 입력이 없는 순수 stateless 발급이다.

핵심 기술 결정:
- 자격증명은 **상태 없이 계산만으로** 만든다. 서버에 저장하지 않으며, TURN 서버가 같은 시크릿으로 password를 재계산해 검증한다(공유 시크릿 방식).
- `username = "{만료_unix_timestamp}:cobrowsing"`, `password = base64(HMAC-SHA1(secret, username))`.
- TTL은 고정 3600초(1시간). 만료 시각은 발급 시점 + TTL.
- 시크릿·URI는 환경변수로 주입하며 개발용 기본값을 갖는다. 시크릿은 응답·로그 어디에도 노출하지 않는다.

> **현행 코드 정렬 주의:** 기존 flat 스펙(`07-turn-credentials.md`)은 huma·`SuccessResponse[T]` 마이그레이션 **이전**에 작성되어 raw gin 핸들러(`func NewTURNHandler()`, `r.GET(...)`, 봉투 없는 JSON, `internal/interfaces/http/turn.go` 단일 파일)를 전제한다. 이 설계도는 flat 스펙의 **TURN 고유 기술 결정(HMAC-SHA1·base64·환경변수·TTL)은 그대로** 옮기되, 트랜스포트 구조는 현재 코드의 실제 컨벤션(huma v2 `Register(api huma.API)`, `SuccessResponse[T]` 봉투, `http/<엔드포인트>` 서브패키지, app 컴포지션 루트 주입)에 맞춰 재표현한다. 형제 핸들러 `internal/interfaces/http/ping`, `internal/interfaces/http/room`과 동일한 형태다.

## Implementation Order

```
[1] Domain Stores                 ✓ (TURN은 의존 안 함)
[2] Room Handler (POST /rooms)    ✓
[3] WebSocket Hub                 ✓
[4] WebSocket Handler             ✓
[5] Signaling Protocol           ✓
[6] Control Event Relay          ✓
[7] TURN Credentials  ← 이 설계
[8] CORS Middleware
```

- **선행 의존성:** 없음 (독립 구현 가능 — 다른 어떤 컴포넌트도 기다리지 않는다)
- **후행 의존성:** 없음

구현 단계(이 기능 내부):

1. (interfaces) `http/turn` 서브패키지에 응답 DTO(`Credentials` + `SuccessResponse[Credentials]`)를 정의한다.
2. (interfaces) `Handler` 구조체 + `NewHandler()` + `Register(api huma.API)` + 핸들러 메서드를 작성한다. HMAC-SHA1 생성·환경변수 로딩은 이 어댑터 안에서 stateless하게 수행한다(서비스/도메인 의존 없음).
3. (app) `New()`에서 `turn.NewHandler()`를 만들어 `NewRouter(...)`에 추가한다.
4. `make openapi`로 `docs/openapi.yaml`을 재생성한다.

## Layer Mapping

| 책임 | 레이어 | 위치 |
|------|--------|------|
| HTTP 오퍼레이션 등록, 응답 DTO, HMAC-SHA1 자격증명 계산, 환경변수 로딩 | **interfaces** | `internal/interfaces/http/turn` |
| 공통 응답 봉투 `SuccessResponse[T]` | **interfaces (base)** | `internal/interfaces/http/dto.go` (기존) |
| 핸들러 생성·라우터 주입 | **app (컴포지션 루트)** | `internal/app/app.go` |

**왜 services/domain 레이어가 없는가:** 이 기능에는 묶을 유즈케이스 흐름도, 보존할 도메인 불변식도, 거쳐야 할 포트도 없다. 자격증명은 환경변수 + 현재 시각으로부터 **계산만으로** 나오는 stateless 값이라 트랜스포트 어댑터(interfaces)에서 직접 처리한다. 형제 `ping` 핸들러가 서비스 의존 없이 트랜스포트 레이어에만 사는 것과 같은 판단이다. depguard 의존 방향(바깥→안쪽)은 위반하지 않는다 — interfaces는 안쪽을 향하지 않는 추가 의존(표준 라이브러리 `crypto/*`, `os`, `time`)만 쓴다.

> 향후 시크릿 로테이션·발급 정책·인증이 붙어 "유즈케이스 흐름"이 생기면 발급 로직을 `services`로, 시크릿 공급을 `domain` 포트로 끌어올리는 리팩터링이 가능하다. MVP에서는 어댑터 내부에 둔다.

## Dependencies

표준 라이브러리 + 기존 의존만 사용한다. **신규 외부 패키지 없음.**

```go
// internal/interfaces/http/turn/turn.go
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
```

> flat 스펙은 `nethttp "net/http"` alias + `github.com/gin-gonic/gin`을 전제했으나, 현재 코드의 huma 서브패키지(`ping`, `room`)는 패키지명이 `turn`이라 `net/http`를 alias 없이 import하고 gin 대신 huma에 등록한다. 그에 맞춘다.

### 환경변수

| 변수명 | 기본값 | 설명 |
|--------|--------|------|
| `TURN_SECRET` | `"changeme"` | HMAC-SHA1 서명 키. **프로덕션 배포 전 반드시 변경**(NFR-3). 응답·로그에 노출 금지(NFR-2). |
| `TURN_URIS` | `"turn:localhost:3478"` | 쉼표(`,`)로 구분한 TURN 서버 URI 목록. 응답 `uris` 배열로 매핑. |

- 기본값은 **MVP 로컬 개발용**이며, 운영 환경별로 설정으로 분리·재구성한다(NFR-3).
- `TURN_SECRET`이 미설정이면 기본값 `"changeme"`로 폴백한다 → 발급은 정상 응답하되 실제 TURN 서버가 없으므로 릴레이 연결은 성립하지 않는다(MVP 허용 동작, AC-8).
- `TURN_URIS`가 미설정이면 기본값 `"turn:localhost:3478"` 한 개를 반환해 항상 **하나 이상의 URI**를 보장한다(FR-3, AC-2).

## Data Structures

### 응답 페이로드 + 봉투

```go
// internal/interfaces/http/turn/dto.go
package turn

import httpbase "co-browsing-session-server/internal/interfaces/http"

type Input struct{} // 입력 없음 — 인증/파라미터 불필요 (FR-7, AC-6)

// Credentials는 TURN 중계 서버 접속 자격증명 페이로드다.
type Credentials struct {
    Username string   `json:"username" doc:"TURN 접속 사용자명. {만료_unix_timestamp}:cobrowsing 형식" example:"1716003600:cobrowsing"`
    Password string   `json:"password" doc:"base64(HMAC-SHA1(secret, username))로 계산된 임시 비밀번호"`
    TTL      int      `json:"ttl" doc:"자격증명 유효 기간(초). 발급 시점 기준" example:"3600"`
    URIs     []string `json:"uris" doc:"접속 대상 TURN 서버 주소 목록(하나 이상)"`
}

// Response는 공통 봉투 SuccessResponse[T]를 입는다 → JSON 상 {"data": {...}}.
type Response = httpbase.SuccessResponse[Credentials]
```

기존 공통 봉투(변경 없음, 참조):

```go
// internal/interfaces/http/dto.go (기존)
type SuccessResponse[T any] struct {
    Body struct {
        Data T `json:"data"`
    }
}
type ErrorResponse = huma.ErrorModel // RFC 7807
```

> flat 스펙은 `TURNCredentials`라는 타입명과 봉투 없는 평면 JSON을 제시했으나, 현재 코드는 모든 성공 응답을 `{"data": ...}` 봉투로 통일했고 패키지명이 `turn`이므로 anti-stutter에 따라 타입명을 `Credentials`(not `TURNCredentials`)로 둔다.

## Interfaces / Contracts

### HTTP Endpoint

```
GET /turn-credentials

요청: 본문/파라미터/인증 없음 (FR-7)

응답 200 OK (SuccessResponse 봉투):
{
  "data": {
    "username": "1716003600:cobrowsing",
    "password": "base64encodedHMACsha1value=",
    "ttl": 3600,
    "uris": [
      "turn:turn.example.com:3478?transport=udp",
      "turn:turn.example.com:3478?transport=tcp",
      "turns:turn.example.com:5349?transport=tcp"
    ]
  }
}
```

### 핸들러 계약 (현행 huma 패턴 — `ping`/`room`과 동일 형태)

```go
// internal/interfaces/http/turn/turn.go
package turn

type Handler struct{} // 의존 없음 (stateless)

func NewHandler() *Handler { return &Handler{} }

// Register는 huma API에 GET /turn-credentials 오퍼레이션을 등록한다.
// httpiface.Handler 인터페이스(Register(api huma.API))를 만족한다 → app에서 NewRouter에 그대로 전달.
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

func (h *Handler) GetCredentials(ctx context.Context, _ *Input) (*Response, error)
```

> flat 스펙의 `func (h *TURNHandler) Register(r *gin.Engine) { r.GET(...) }`(raw gin)은 huma 마이그레이션 전 형태다. 현재 코드의 HTTP 핸들러는 모두 huma 타입드 핸들러(`Register(api huma.API)`)로 등록되고, raw gin 직접 등록은 WebSocket(`/ws`)에만 쓴다. TURN은 raw 업그레이드가 필요 없으므로 huma 경로를 따른다.

## Behavior

### HMAC-SHA1 임시 자격증명 생성 (Coturn 호환)

`GetCredentials`는 입력 없이 호출되어 다음을 stateless하게 수행한다:

```go
const ttlSeconds = 3600

func (h *Handler) GetCredentials(ctx context.Context, _ *Input) (*Response, error) {
    // 1. 만료 시각 = 현재 시각 + TTL (발급 시점 기준)
    expiry := time.Now().Unix() + ttlSeconds

    // 2. username = "{만료_unix_timestamp}:cobrowsing"
    username := fmt.Sprintf("%d:cobrowsing", expiry)

    // 3. HMAC-SHA1 서명 → base64
    secret := turnSecret() // os.Getenv("TURN_SECRET") or "changeme"
    mac := hmac.New(sha1.New, []byte(secret))
    mac.Write([]byte(username))
    password := base64.StdEncoding.EncodeToString(mac.Sum(nil))

    // 4. 봉투에 담아 반환
    response := &Response{}
    response.Body.Data = Credentials{
        Username: username,
        Password: password,
        TTL:      ttlSeconds,
        URIs:     turnURIs(), // TURN_URIS 파싱 or 기본값
    }
    return response, nil
}
```

동작 규칙:

- **TTL / 만료**(FR-4, FR-5, AC-3, AC-4): `ttl=3600`을 응답에 명시한다. `username`에 인코딩된 만료 timestamp가 곧 유효 기간의 경계다. 만료 후 같은 username으로 계산한 password는 TURN 서버가 만료된 것으로 보고 거부한다(검증·만료 enforcement는 TURN 서버 측 책임, 서버는 만료 시각만 발급). 만료 시각은 **발급 시점부터 계산**되며 저장하지 않는다.
- **매 요청 새 자격증명**(AC-5): timestamp가 초 단위로 들어가므로 1초 이상 간격을 둔 두 요청은 서로 다른 `username`/`password`/만료 시각을 받는다. 같은 초에 발급된 것들은 동일 만료를 향한다.
- **인증 불필요**(FR-7, AC-6): `Input`이 빈 구조체라 파라미터·본문·인증 헤더를 요구하지 않는다.
- **세션과 독립**(FR-6, NFR-4): RoomSession/Invitation 저장소, Hub, Coordinator 어디에도 접근하지 않는다 → 다른 기능의 부하·장애와 무관.

### 환경변수 로딩 · 기본값 · 미설정 시 동작

```go
const (
    defaultSecret = "changeme"
    defaultURI    = "turn:localhost:3478"
)

func turnSecret() string {
    if s := os.Getenv("TURN_SECRET"); s != "" {
        return s
    }
    return defaultSecret // MVP 개발용 폴백
}

func turnURIs() []string {
    raw := os.Getenv("TURN_URIS")
    if raw == "" {
        return []string{defaultURI} // 항상 하나 이상 보장 (FR-3, AC-2)
    }
    parts := strings.Split(raw, ",")
    uris := make([]string, 0, len(parts))
    for _, part := range parts {
        if trimmed := strings.TrimSpace(part); trimmed != "" {
            uris = append(uris, trimmed)
        }
    }
    if len(uris) == 0 {
        return []string{defaultURI} // 공백만 들어와도 하나 이상 보장
    }
    return uris
}
```

- **기본값**(NFR-3): `TURN_SECRET="changeme"`, `TURN_URIS="turn:localhost:3478"`. MVP 로컬용이며 운영 배포 전 환경변수로 재설정한다.
- **TURN 서버 미설정 시**(AC-8, 비즈니스 룰): 시크릿이 기본값이거나 실제 TURN 서버가 없어도 **발급 응답은 정상 200**. 다만 릴레이 연결 자체는 성립하지 않는다 — P2P가 성공하는 한 화면 공유에 영향 없음(폴백 전용, AC-7).

### 보안 (NFR-1, NFR-2)

- **단기성**(NFR-1, AC-4): 영구 자격증명을 발급하지 않는다. 매 요청마다 발급 시점 기준 TTL(3600초)을 갖는 새 값만 만든다.
- **시크릿 비노출**(NFR-2): `TURN_SECRET`은 응답 본문(`Credentials`)에 **포함하지 않는다** — `password`는 시크릿으로 서명한 결과(HMAC)일 뿐 시크릿 원문이 아니다. 시크릿은 어떤 경로로도 클라이언트에 나가지 않는다.
- **비로깅**(NFR-2): 핸들러는 시크릿·발급된 `password`·`username`을 로그에 남기지 않는다. 이 경로에는 로깅을 두지 않으며(에러도 없는 stateless 계산), 로깅이 필요하면 민감값을 제외한 메타데이터만 남긴다. CLAUDE.md의 "민감정보(시리얼 코드, 토큰) 비로깅" 규칙을 따른다.
- **AC-7(폴백 전용)**: 직접 연결(P2P) 성공 시 발급된 자격증명이 사용되지 않아도 화면 공유는 정상 — 서버는 발급만 책임지고 사용 여부를 강제하지 않는다.

## File Locations

| 작업 | 파일 | 비고 |
|------|------|------|
| 신규 | `internal/interfaces/http/turn/turn.go` | `Handler`, `NewHandler`, `Register(api huma.API)`, `GetCredentials`, `turnSecret`/`turnURIs` 헬퍼 |
| 신규 | `internal/interfaces/http/turn/dto.go` | `Input`, `Credentials`, `Response = SuccessResponse[Credentials]` |
| 신규(테스트) | `internal/interfaces/http/turn/turn_test.go` | 핸들러 단위 테스트 |
| 수정 | `internal/app/app.go` | `New()`에서 `turn.NewHandler()`를 `NewRouter(...)`에 추가 |
| 재생성 | `docs/openapi.yaml` | `make openapi` (수기 편집 금지) |

> flat 스펙은 단일 파일 `internal/interfaces/http/turn.go`를 제시했으나, 현재 코드는 엔드포인트를 **서브패키지**(`http/ping`, `http/room`)로 분리하고 각 패키지에 `Handler` + `Register` + `dto.go`를 둔다. 그 컨벤션에 맞춰 `http/turn/` 서브패키지로 둔다.

### app 배선 (현행 형태 반영)

```go
// internal/app/app.go — New() 내부, interfaces(HTTP) 조립부에 추가
router := httpiface.NewRouter(
    room.NewHandler(roomSessionService),
    ping.NewHandler(),
    turn.NewHandler(), // ← 추가 (import "co-browsing-session-server/internal/interfaces/http/turn")
)
```

## Test Plan

테스트 컨벤션은 `golang-testing` 스킬을 따른다. 테이블 기반 + named subtest, 독립 케이스는 `t.Parallel()`.

### 단위 테스트 (`internal/interfaces/http/turn/turn_test.go`)

huma 타입드 핸들러는 `humatest`(또는 조립된 라우터에 `httptest`로 요청)로 검증한다. 환경변수는 `t.Setenv`로 격리한다.

| 케이스 | 검증 (대응 AC) |
|--------|----------------|
| 기본 발급 | 200 OK + `data.username`/`password`/`ttl`/`uris` 모두 존재 (AC-1) |
| URI 최소 보장 | `TURN_URIS` 미설정 시 `uris` 길이 ≥ 1 (FR-3, AC-2) |
| URI 파싱 | `TURN_URIS="a,b,c"` → `uris == ["a","b","c"]`, 공백 트림 |
| TTL 명시 | `data.ttl == 3600` (FR-4, AC-3) |
| username 형식 | `^\d+:cobrowsing$`, 숫자부 = 현재 unix + 3600 근방 |
| HMAC 정확성 | `t.Setenv("TURN_SECRET","k")` 후 `password == base64(HMAC_SHA1("k", username))` 재계산과 일치 |
| 연속 호출 차이 | 1초 간격(또는 timestamp mock) 두 요청의 username/만료가 다를 수 있음 (AC-5) |
| 인증 불필요 | 헤더·본문 없이 요청해도 200 (FR-7, AC-6) |
| 시크릿 비노출 | 응답 JSON 어디에도 `TURN_SECRET` 원문(`"changeme"`/설정값)이 나타나지 않음 (NFR-2) |

### 통합 테스트 (`internal/app/integration_test.go` 보강)

기존 `newTestServer`(실제 `app.New().Engine()` 조립) 패턴 재사용:

| 케이스 | 검증 (대응 AC) |
|--------|----------------|
| 라우팅·조립 | `GET /turn-credentials` → 200 + `{"data":{...}}` 봉투, 모든 필드 포함 (AC-1, AC-8) |
| 미구성 환경 | 환경변수 없이도(기본값) 발급 정상 200 (AC-8) |

### OpenAPI 드리프트

- DTO/오퍼레이션 추가 후 `make openapi`로 `docs/openapi.yaml` 재생성, `make openapi-check`(CI)로 드리프트 차단.

## Traceability

### 기능 요구사항 (FR)

| ID | 충족 위치 |
|----|-----------|
| FR-1 (자격증명 발급) | `GetCredentials` — `GET /turn-credentials` 200 응답 |
| FR-2 (username/password + URI 목록 + TTL 포함) | `Credentials{Username, Password, URIs, TTL}` |
| FR-3 (URI 하나 이상) | `turnURIs()` — 미설정/공백 시 `defaultURI` 폴백으로 ≥1 보장 |
| FR-4 (명시적 유효 기간) | `TTL: 3600` + `username`에 만료 timestamp 인코딩 |
| FR-5 (만료 후 사용 불가) | 만료 timestamp 발급 → TURN 서버가 만료분 거부(검증은 TURN 측) |
| FR-6 (독립 발급) | services/domain/Hub 무의존 stateless 핸들러 (Layer Mapping) |
| FR-7 (인증 불필요, MVP) | `Input struct{}` — 파라미터·인증 없음 |

### 비기능 요구사항 (NFR)

| ID | 충족 위치 |
|----|-----------|
| NFR-1 (단기성·만료) | 고정 TTL 3600s, 영구 자격증명 미발급 — Behavior/보안 |
| NFR-2 (시크릿·비밀번호 비노출/비로깅) | 시크릿 응답 제외 + 핸들러 비로깅 — 보안 |
| NFR-3 (운영별 설정 분리·기본값 비사용) | `TURN_SECRET`/`TURN_URIS` 환경변수 + 개발용 기본값 명시 — Dependencies/환경변수 |
| NFR-4 (세션·실시간 연결과 독립) | 저장소/Hub/Coordinator 무의존 — Layer Mapping |

### 인수 조건 (AC)

| ID | 충족 위치 |
|----|-----------|
| AC-1 (모든 필드 정상 발급) | 단위·통합 "기본 발급" 테스트 + `GetCredentials` |
| AC-2 (URI ≥ 1) | `turnURIs()` 폴백 + "URI 최소 보장" 테스트 |
| AC-3 (명시적 유효 기간) | `TTL: 3600` + "TTL 명시" 테스트 |
| AC-4 (만료 후 접속 불가) | 만료 timestamp 발급(검증 TURN 측) — Behavior TTL/만료 |
| AC-5 (간격 둔 두 요청 상이) | timestamp 기반 username + "연속 호출 차이" 테스트 |
| AC-6 (무인증 발급) | `Input struct{}` + "인증 불필요" 테스트 |
| AC-7 (P2P 성공 시 미사용도 정상) | 서버는 발급만, 사용 강제 안 함 — Behavior/보안 |
| AC-8 (TURN 미구성 환경도 발급 정상) | 기본값 폴백 + "미구성 환경" 통합 테스트 |

**누락 0** — FR-1~7, NFR-1~4, AC-1~8 전부 매핑됨.
