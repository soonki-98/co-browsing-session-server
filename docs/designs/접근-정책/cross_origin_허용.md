# 교차 출처 요청 허용(CORS) — 기술 설계도

> 기획: [docs/specs/접근-정책/cross_origin_허용.md](../../specs/접근-정책/cross_origin_허용.md)

## Overview

웹 콘솔(상담원 프론트엔드)의 출처에서 오는 Cross-Origin HTTP 요청을 허용하는 Gin 미들웨어다. 운영자가 환경변수로 지정한 허용 출처 목록(allow-list)을 기준으로 출처를 **정확 일치**로 검사하고, 일치하면 요청한 출처를 그대로 `Access-Control-Allow-Origin`에 반사한다. 와일드카드(`*`)는 쓰지 않으며, 자격 정보를 동반한 교차 출처 요청을 위해 `Access-Control-Allow-Credentials: true`를 항상 내려준다. Preflight(OPTIONS)는 본 핸들러를 타지 않고 즉시 응답한다.

외부 CORS 패키지(`gin-contrib/cors` 등)는 쓰지 않고 직접 구현한다 — 설정이 단순하여 직접 구현이 더 명확하다.

이 미들웨어는 `internal/interfaces/http`의 라우터 조립 시 모든 라우트보다 먼저 `Use`로 등록되어, 세션 서버의 모든 HTTP API에 일관되게 적용된다. WebSocket 업그레이드(`GET /ws`)는 브라우저의 표준 CORS 차단 대상이 아니므로 이 정책으로 막히거나 영향받지 않는다.

## Implementation Order

CORS 미들웨어는 다른 컴포넌트와 **독립적**이다(선행/후행 의존성 없음).

1. `internal/interfaces/http/middleware/cors.go` 신규 작성 — `LoadAllowedOrigins()` + `CORSMiddleware(allowedOrigins []string) gin.HandlerFunc`
2. `internal/interfaces/http/router.go` 수정 — 기존 `middleware.Default()` 등록 직후 `middleware.CORSMiddleware(middleware.LoadAllowedOrigins())`를 `Use`로 추가

## Layer Mapping

| 레이어 | 디렉터리 | 이 기능에서의 역할 |
|--------|----------|--------------------|
| **interfaces** | `internal/interfaces/http/middleware` | CORS 미들웨어 본체(출처 검사·응답 헤더·preflight 처리)와 환경변수 로딩. HTTP 트랜스포트 경계의 횡단 관심사다. |
| **interfaces** | `internal/interfaces/http` | `NewRouter`에서 미들웨어를 조립(`Use`)한다. |

순수 트랜스포트 관심사이므로 `domain`/`services`/`infrastructure`에는 추가 코드가 없다. 미들웨어는 표준 라이브러리(`net/http`, `os`, `strings`)와 Gin에만 의존하므로 depguard의 안쪽 방향 의존 규칙을 위반하지 않는다(interfaces → 바깥 프레임워크/표준 라이브러리만 사용).

## Dependencies

표준 라이브러리와 Gin만 사용한다. 추가 외부 패키지 없음.

```go
import (
    "net/http"
    "os"
    "strings"

    "github.com/gin-gonic/gin"
)
```

### 환경변수

| 변수 | 형식 | 기본값 | 설명 |
|------|------|--------|------|
| `CORS_ALLOWED_ORIGINS` | 쉼표 구분 출처 목록 | `http://localhost:3000` | 허용할 웹 콘솔 출처 목록. 예: `https://console.example.com,http://localhost:3000`. 미설정 또는 빈 문자열이면 로컬 개발용 기본값 사용. |

운영 환경별(로컬/스테이징/운영) 출처는 코드 변경 없이 이 환경변수로만 바꾼다.

## Data Structures

추가 데이터 구조 없음. 허용 출처는 `[]string`으로 표현하고, 미들웨어 클로저가 이를 캡처한다.

## Interfaces / Contracts

```go
// internal/interfaces/http/middleware/cors.go
package middleware

// LoadAllowedOrigins는 환경변수 CORS_ALLOWED_ORIGINS(쉼표 구분)에서 허용 출처를 읽는다.
// 미설정이면 로컬 개발용 기본값을 반환한다. router.go에서 호출하므로 export한다.
func LoadAllowedOrigins() []string

// CORSMiddleware는 allowedOrigins를 기준으로 교차 출처 요청을 처리하는 Gin 미들웨어를 반환한다.
// allowedOrigins: 허용할 출처 목록(보통 LoadAllowedOrigins()의 결과).
func CORSMiddleware(allowedOrigins []string) gin.HandlerFunc
```

라우터 조립 계약(기존 `Default()`와 동일한 `middleware` 패키지, 동일한 `Use` 위치):

```go
// internal/interfaces/http/router.go
func NewRouter(handlers ...Handler) *gin.Engine {
    engine := gin.New()
    engine.Use(middleware.Default()...)                                       // 기존 logger + recovery
    engine.Use(middleware.CORSMiddleware(middleware.LoadAllowedOrigins()))    // 신규 CORS

    api := humagin.New(engine, huma.DefaultConfig("Co-Browsing Session Server", "1.0.0"))
    for _, handler := range handlers {
        handler.Register(api)
    }
    return engine
}
```

미들웨어는 huma API 마운트(라우트 등록)보다 먼저 등록해야 모든 엔드포인트에 적용된다.

## Behavior

### 허용 출처 로드 (`LoadAllowedOrigins`)

```go
func LoadAllowedOrigins() []string {
    raw := os.Getenv("CORS_ALLOWED_ORIGINS")
    if raw == "" {
        return []string{"http://localhost:3000"}
    }
    return strings.Split(raw, ",")
}
```

- `CORS_ALLOWED_ORIGINS` 미설정/빈 문자열 → 기본값 `["http://localhost:3000"]` (FR-5, AC-5).
- 쉼표로 구분된 여러 출처를 각각 독립 허용 항목으로 만든다 (FR-6, AC-4).

### 미들웨어 동작 (`CORSMiddleware`)

모든 요청에 대해 다음 순서로 처리한다.

```
1. 요청 Origin 헤더(c.Request.Header.Get("Origin")) 추출
2. allowedOrigins 목록과 "정확 일치" 검사(프로토콜·도메인·포트 전부 동일)
   - 일치: Access-Control-Allow-Origin = 요청한 그 Origin 값(반사)   // 와일드카드 미사용
   - 불일치 또는 Origin 없음: Allow-Origin 헤더를 설정하지 않음(브라우저가 차단)
3. 출처가 허용된 경우에만 함께 내려주는 헤더:
   Access-Control-Allow-Methods:     GET, POST, OPTIONS
   Access-Control-Allow-Headers:     Content-Type, Authorization
   Access-Control-Allow-Credentials: true
4. Preflight(메서드 == OPTIONS):
   → 204 No Content로 즉시 응답하고 이후 핸들러 실행을 중단(c.AbortWithStatus)
5. 그 외 요청: c.Next()로 다음 핸들러 진행
```

핵심 결정:

- **정확 일치 + 반사**: 허용 목록에 든 출처와 문자열 정확 일치할 때만, 그 출처 문자열을 그대로 `Access-Control-Allow-Origin`에 넣는다. 부분 일치·하위 도메인 자동 포함 없음 (규칙: 정확 일치, AC-1).
- **와일드카드 미사용**: `*`를 절대 쓰지 않는다. `Access-Control-Allow-Credentials: true`와 `*`는 함께 쓸 수 없고, 자격 정보 동반 요청을 정상 처리하려면 출처를 정확히 가리켜야 한다 (규칙, AC-6, FR-2).
- **불허 출처는 무표시 통과**: 허용 목록에 없으면 Allow-Origin 헤더를 붙이지 않고 요청을 그대로 통과시킨다. 서버가 차단하는 게 아니라 신뢰 표시를 빼서 브라우저가 응답을 차단하게 둔다 (FR-3, AC-2).
- **Origin 없는 요청**: 브라우저가 아닌 호출 등 Origin 헤더가 없는 요청은 허용/차단 판단 대상이 아니므로 헤더 추가 없이 정상 처리한다(규칙).
- **Preflight 즉시 응답**: OPTIONS 요청은 본 핸들러를 진행하지 않고 204로 즉시 응답한다 (FR-4, AC-3).
- **환경변수·기본값**: 허용 목록은 `CORS_ALLOWED_ORIGINS`로 주입하고, 미설정 시 로컬 개발 기본값(`http://localhost:3000`)을 쓴다 (FR-5, FR-6, NFR-1).
- **전 엔드포인트 일관 적용**: 라우트 등록 전에 `Use`로 붙어 모든 HTTP API에 동일하게 적용된다 (NFR-2).
- **신뢰 기준의 단일성**: 신뢰 판단의 유일한 근거는 운영자가 명시한 허용 목록이며, 요청 측이 스스로를 신뢰 대상으로 만들 수 없다 (NFR-3).

### WebSocket과 CORS

`GET /ws`는 WebSocket 업그레이드 요청이라 브라우저가 `Origin` 헤더를 보낸다. 그러나 WebSocket 핸드셰이크는 브라우저의 표준 CORS 차단 대상이 아니므로 이 미들웨어가 응답 헤더를 설정해도 업그레이드 자체에는 영향이 없다(규칙: WebSocket 연결 수립은 이 정책에 영향받지 않음).

실제 코드에서 gorilla/websocket Upgrader의 `CheckOrigin`은 항상 `true`를 반환하며, 주석으로 출처 검증 책임이 이 CORS 미들웨어에 있음을 명시한다.

```go
// internal/interfaces/ws/ws.go
// Origin 검증은 Step 8의 CORS 미들웨어가 담당하므로 여기서는 모두 허용한다.
var upgrader = websocket.Upgrader{
    CheckOrigin: func(_ *nethttp.Request) bool { return true },
}
```

이 미들웨어는 주로 `/serial_number`, `/turn-credentials` 같은 일반 HTTP 엔드포인트를 위한 것이다.

## File Locations

| 작업 | 파일 |
|------|------|
| 신규 생성 | `internal/interfaces/http/middleware/cors.go` (`LoadAllowedOrigins`, `CORSMiddleware`) |
| 수정 | `internal/interfaces/http/router.go` (`NewRouter`에 `Use(middleware.CORSMiddleware(...))` 추가) |
| 관련(기존) | `internal/interfaces/http/middleware/recovery_logger.go` (`middleware.Default()` — 동일 패키지·등록 위치 기준) |
| 관련(기존) | `internal/interfaces/ws/ws.go` (`upgrader.CheckOrigin` 항상 허용; 출처 검증은 CORS 미들웨어 책임) |

## Test Plan

`internal/interfaces/http/middleware` 패키지의 테이블 기반 테스트로 미들웨어 동작을, `LoadAllowedOrigins`는 환경변수 분기로 검증한다(`golang-testing` 표준: `t.Run` named subtest, 독립 케이스 `t.Parallel()`). 관찰 가능한 동작(응답 헤더·상태 코드)만 검증한다.

| 테스트 | 검증 내용 | 매핑 |
|--------|-----------|------|
| 허용 출처 반사 | 허용 목록에 `http://localhost:3000`이 있고 그 Origin으로 요청 시 `Access-Control-Allow-Origin: http://localhost:3000` 응답 | FR-1·FR-2, AC-1 |
| 불허 출처 무표시 | 허용 목록에 없는 Origin 요청 시 `Access-Control-Allow-Origin` 헤더 없음 | FR-3, AC-2 |
| Preflight 즉시 응답 | `OPTIONS` 요청(예: `OPTIONS /serial_number`) → 204, 본 핸들러 미실행 | FR-4, AC-3 |
| 다중 출처 | `CORS_ALLOWED_ORIGINS=https://a.example.com,https://b.example.com` 시 두 출처 각각 허용 | FR-6, AC-4 |
| 기본값 | 환경변수 미설정 시 `LoadAllowedOrigins()`가 `["http://localhost:3000"]` 반환, 그 출처 허용 | FR-5, AC-5 |
| 와일드카드 미사용 + Credentials | 허용 출처 요청 시 Allow-Origin이 `*`가 아닌 그 출처, `Access-Control-Allow-Credentials: true` 동반 | AC-6, 규칙 | 

## Traceability

### 기능 요구사항 (FR)

| ID | 요구 | 반영 |
|----|------|------|
| FR-1 | 운영자 지정 허용 목록 기준으로 허용 판단 | `CORSMiddleware`의 정확 일치 검사 + `LoadAllowedOrigins` |
| FR-2 | 허용 출처에 신뢰 표시(Allow-Origin) 응답 | Behavior 2단계: 일치 시 Origin 반사 |
| FR-3 | 불허 출처에는 신뢰 표시 미포함 | Behavior 2단계: 불일치 시 헤더 미설정, 무표시 통과 |
| FR-4 | Preflight 즉시 응답·본 처리 안 함 | Behavior 4단계: OPTIONS → 204 + Abort |
| FR-5 | 운영 환경별 지정, 기본은 로컬 개발 출처 | `LoadAllowedOrigins` 환경변수 + 기본값 |
| FR-6 | 다중 허용 출처 각각 허용 | `strings.Split`로 목록화, 각 항목 독립 일치 |

### 규칙 / 제약 (비즈니스 룰)

| 규칙 | 반영 |
|------|------|
| 정확 일치(프로토콜·도메인·포트 동일, 부분/하위도메인 금지) | Behavior 2단계 정확 일치 |
| 전체 와일드카드 미사용 | Allow-Origin은 항상 요청 출처 반사, `*` 금지 |
| 자격 정보 동반 교차 출처 정상 처리(출처 그대로 반사) | `Allow-Credentials: true` + Origin 반사 |
| WebSocket 연결 수립은 이 정책에 영향 없음 | "WebSocket과 CORS" 절, `CheckOrigin` 항상 true |
| Origin 없는 요청은 판단 대상 아님·정상 처리 | Behavior 2단계: Origin 없으면 헤더 미추가 통과 |

### 비기능 요구사항 (NFR)

| ID | 요구 | 반영 |
|----|------|------|
| NFR-1 | 환경별 설정으로 코드 변경 없이 변경 | `CORS_ALLOWED_ORIGINS` 환경변수 |
| NFR-2 | 모든 API에 일관 적용 | 라우트 등록 전 `Use`로 전역 적용 |
| NFR-3 | 신뢰 기준은 명시 목록뿐, 임의 출처 자가 신뢰 불가 | 허용 목록 외 출처 무표시; 클라이언트가 기준을 바꿀 수 없음 |

### 인수 조건 (AC)

| ID | 검증 |
|----|------|
| AC-1 | Test Plan "허용 출처 반사" |
| AC-2 | Test Plan "불허 출처 무표시" |
| AC-3 | Test Plan "Preflight 즉시 응답" |
| AC-4 | Test Plan "다중 출처" |
| AC-5 | Test Plan "기본값" |
| AC-6 | Test Plan "와일드카드 미사용 + Credentials" |

누락 없음: 기획 스펙의 FR-1~6, 규칙 5개, NFR-1~3, AC-1~6 전부 매핑됨.
