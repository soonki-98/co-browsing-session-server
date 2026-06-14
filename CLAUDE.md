# CLAUDE.md

이 저장소에서 작업할 때 지켜야 할 규칙을 정의한다. 작성 기준은 **Go·레이어드(클린) 아키텍처의 베스트 프랙티스**이며, 명시적으로 예외를 둔 곳(예: 네이밍)만 프로젝트 고유 규칙을 따른다.

> 각 레이어의 **개념·역할**은 해당 디렉터리의 `README.md`에서 설명한다. 이 문서와 각 레이어의 `CLAUDE.md`는 **작업 시 지킬 규칙(가드레일)**에 집중한다.

## 프로젝트 개요

co-browsing 세션 서버 — 고객지원 상담원이 고객 화면을 함께 보며 안내하는 화면 공유 백엔드. 시리얼 코드로 고객 세션을 만들고, 상담원이 합류하면 WebRTC 시그널링과 제어 이벤트를 중계한다. 전체 요구사항은 `docs/requirements.md` 참조.

**스택**: Go · Gin(HTTP) · huma v2(코드-퍼스트 OpenAPI) · gorilla/websocket · google/uuid

## 명령어

| 목적 | 명령 |
|------|------|
| 핫리로드 개발 서버 | `make dev` (air) |
| 전체 테스트 | `go test ./...` |
| 레이스 검사 포함 테스트 | `go test -race ./...` |
| 린트(아키텍처 의존 규칙 포함) | `golangci-lint run` |
| OpenAPI 스펙 재생성 | `make openapi` |
| OpenAPI 드리프트 검사(CI) | `make openapi-check` |

커밋 전 최소 `go test ./...`와 `golangci-lint run`을 통과시킨다. 엔드포인트/DTO를 바꾼 경우 `make openapi`로 `docs/openapi.yaml`을 재생성해 함께 커밋한다.

## 아키텍처 — 레이어드 의존 규칙

이 프로젝트는 클린 아키텍처의 **의존 규칙(Dependency Rule)**을 따른다: **의존은 항상 안쪽(도메인)을 향한다.** 바깥 레이어는 안쪽을 알지만, 안쪽은 바깥을 모른다. 레이어 경계는 인터페이스(포트)로 끊고, 구체 구현은 컴포지션 루트에서 주입한다(의존성 역전, 포트 & 어댑터).

```
        ┌─────────────────────────── app (컴포지션 루트) ───────────────────────────┐
        │   interfaces ──▶ services ──▶ domain ◀── infrastructure                     │
        │   (어댑터)        (유즈케이스)   (핵심)      (포트 구현)                       │
        └────────────────────────────────────────────────────────────────────────────┘
```

| 레이어 | 디렉터리 | import 허용 | import 금지 |
|--------|----------|-------------|-------------|
| **domain** | `internal/domain` | (표준 라이브러리·범용 유틸만) | 그 외 모든 내부 레이어 |
| **services** | `internal/services` | domain | interfaces, infrastructure(구현), app |
| **interfaces** | `internal/interfaces` | services, domain | infrastructure(구현), app |
| **infrastructure** | `internal/infrastructure` | domain | services, interfaces, app |
| **app** | `internal/app` | 전부(조립 전용) | — |

이 규칙은 `.golangci.yml`의 **depguard**가 강제한다. 위반 시 빌드가 아니라 린트에서 잡힌다 — 임포트를 추가하기 전에 "이 의존이 안쪽을 향하는가"를 먼저 확인한다.

### 새 코드를 어느 레이어에 둘지

- **기술과 무관한 규칙·불변식·식별자**(엔티티, 값 객체, 도메인 에러, 포트 인터페이스) → `domain`
- **여러 도메인/포트를 묶는 유즈케이스 흐름**(트랜잭션 경계, 오케스트레이션) → `services`
- **외부 프로토콜 ↔ 유즈케이스 변환**(HTTP 핸들러, WebSocket 어댑터, DTO) → `interfaces`
- **포트의 구체 구현**(저장소, 외부 시스템 클라이언트) → `infrastructure`
- **구체 타입 생성·주입·라우팅 조립** → `app`

판단이 애매하면, 그 코드가 의존하는 것이 무엇인지 본다. 프레임워크/DB를 알아야 하면 안쪽(domain/services)이 아니다.

## 네이밍 / 코딩 컨벤션

### 네이밍 — 풀네임 (프로젝트 규칙, 의도적 결정)

> Go 관용(짧은 receiver `h`, `svc`)을 **의도적으로 오버라이드**한다. 가독성과 일관성을 위해 이 프로젝트는 풀네임을 쓴다. 새 코드도 반드시 따른다.

- receiver는 타입을 풀네임으로: `func (handler *Handler)`, `func (roomSessionRepository *RoomSessionRepository)` — `h`, `r`, `repo` 금지
- 변수도 의미를 드러내는 풀네임: `roomID`, `serialNumberGenerator`, `resolvedInvitation` — `id`, `gen`, `inv` 금지
- 약어는 통용되는 것만 그대로(`ID`, `HTTP`, `URL`, `TTL`), 그 외는 풀어 쓴다
- 패키지명은 짧은 소문자 단수형, 언더스코어/복수형 금지(`roomsession`, `hub`) — 이건 Go 표준과 일치

### 에러 처리

- 도메인 경계의 분기 가능한 에러는 **sentinel 에러**로 선언하고 `errors.Is`로 판별한다
  - `var ErrNotFound = errors.New("...")` / `if errors.Is(err, ErrNotFound) { ... }`
- 에러를 위로 올릴 때는 **맥락을 더해 래핑**한다: `fmt.Errorf("resolve invitation: %w", err)` (`%w`로 체인 보존)
- 레이어 경계마다 에러를 그 레이어의 언어로 번역한다. 안쪽 sentinel을 바깥이 직접 알지 않도록, 유즈케이스 경계에서 한 번 변환한다(예: 도메인 에러 → 유즈케이스 에러 → 트랜스포트 상태 코드)
- 에러 타입이 필요하면 `errors.As`. 패닉은 복구 불가한 프로그래밍 오류에만 — 흐름 제어에 쓰지 않는다

### 로깅

- 표준 `log/slog`(구조적 로깅)를 기준으로 한다. 키-값으로 맥락을 남기고, 문자열 보간 로그를 피한다
- 로그는 **경계에서 최소한으로**. 핸들러/어댑터처럼 흐름이 끝나는 지점에서 한 번 남기고, 안쪽 레이어는 에러를 반환할 뿐 로깅하지 않는다(이중 로깅 방지)
- 민감정보(시리얼 코드, 토큰)는 로그에 남기지 않는다

### 일반 Go 베스트 프랙티스

- **생성자 주입**: 의존성은 `NewXxx(deps...)` 생성자 인자로 받는다. 전역 상태·패키지 레벨 가변 변수 금지
- **인터페이스는 받고, 구조체는 반환**(accept interfaces, return structs). 포트는 그 포트를 **쓰는 쪽**의 필요에 맞춰 작게 정의한다
- `context.Context`는 요청 흐름 함수의 **첫 번째 인자**로 전달한다(`ctx context.Context`). 구조체에 저장하지 않는다
- 공유 가변 상태는 동시성 안전하게(뮤텍스/채널) 보호한다. `go test -race`로 검증한다
- 작은 단위로 분리한다 — 한 파일/타입이 한 가지 책임만. 파일이 커지면 책임이 섞였다는 신호다

## 테스트

- **테이블 기반 테스트**를 기본으로 한다. 케이스는 `name`으로 식별하고 `t.Run`으로 묶는다
- 동시성 코드는 `go test -race ./...`로 검증한다(Hub 등)
- 통합 테스트는 `httptest.Server` + 실제 in-memory 구현으로 엔드투엔드 흐름을 확인한다 — 과한 목(mock)보다 실제 조립을 선호한다
- 단언 실패 메시지는 "무엇이 기대였는지"를 한국어로 명확히 적는다

## OpenAPI (코드-퍼스트)

스펙은 **코드에서 생성**한다(`cmd/gen-openapi`). `docs/openapi.yaml`을 손으로 고치지 않는다. huma 핸들러/DTO를 바꾸면 `make openapi`로 재생성하고, `make openapi-check`가 드리프트를 막는다.

## 레이어별 규칙

작업하는 레이어의 `CLAUDE.md`를 먼저 읽는다.

- [`internal/domain/CLAUDE.md`](internal/domain/CLAUDE.md) — 도메인(핵심)
- [`internal/services/CLAUDE.md`](internal/services/CLAUDE.md) — 유즈케이스
- [`internal/interfaces/CLAUDE.md`](internal/interfaces/CLAUDE.md) — HTTP/WebSocket 어댑터
- [`internal/infrastructure/CLAUDE.md`](internal/infrastructure/CLAUDE.md) — 포트 구현
- [`internal/app/CLAUDE.md`](internal/app/CLAUDE.md) — 컴포지션 루트
