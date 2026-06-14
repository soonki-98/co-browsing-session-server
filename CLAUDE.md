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

## 개발 워크플로 — 기획 → 기술 설계 → 구현(TDD) → 검증

기능 개발은 **두 단계 문서**를 거쳐 **TDD**로 구현한다. 각 단계는 전용 스킬·에이전트가 담당한다(`.claude/skills/`, `.claude/agents/`). 이 문서들은 규칙을 재서술하지 않고 이 `CLAUDE.md`와 레이어 `CLAUDE.md`를 단일 출처로 참조한다.

1. **기획 요구사항** — `docs/specs/<요구사항명>/<세부요구사항>.md`. *무엇을*: 사용자 관찰 행동·비즈니스 규칙·번호가 매겨진 FR/AC. 기술 용어 없음. → 스킬 `spec-authoring`, 에이전트 `spec-writer`.
2. **기술 설계도** — `docs/designs/<요구사항명>/<세부요구사항>.md`(기획과 같은 경로 미러). *어떻게*: 레이어 배치·자료구조·포트·에러/동시성·File Locations·Test Plan + 기획 FR/AC로 되돌리는 Traceability. → 스킬 `tech-design-authoring`, 에이전트 `tech-designer`.
3. **구현(TDD)** — 설계도를 입력으로, 유닛마다 `test-writer`(실패 테스트 RED) → `<layer>-implementer`(최소 구현 GREEN) → 필요 시 `refactorer`. 여러 레이어가 걸치면 `implementation-orchestrator`가 레이어·의존순서·위임 계획을 산출한다(subagent는 중첩 불가 — 실제 디스패치는 메인 스레드). → 스킬 `spec-implementation`, `test-authoring`.
4. **검증** — `verifier`가 자율 루프(`/loop code-verification`)로 build/test/race/lint/openapi 게이트를 완료조건까지 반복. → 스킬 `code-verification`.

```
spec-writer ─▶ tech-designer ─▶ [orchestrator] ─▶ test-writer(RED) ─▶ <layer>-implementer(GREEN) ─▶ refactorer ─▶ verifier(/loop)
 docs/specs      docs/designs                         *_test.go            internal/<layer>
```

> 범용 Go 품질의 "어떻게"(네이밍·에러·동시성·테스트 등)는 아래 "Go 품질 스킬"의 `golang-*` 스킬을 따른다. 충돌 시 cc 스킬 우선.

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

### 네이밍 — Go 관용 (`golang-naming` 스킬 기준)

> 네이밍은 **`golang-naming` 스킬**(samber/cc-skills-golang)을 표준으로 따른다. (이전의 "풀네임 오버라이드" 규칙은 **폐기** — cc-skills-golang 기준으로 정렬했다. 기존 코드의 풀네임 식별자는 만질 때 점진적으로 관용 네이밍으로 리네임한다.)

핵심만:
- receiver는 1~2자 약어로 일관되게: `func (s *Server)`, `func (h *Handler)` — `this`/`self` 금지, 메서드마다 이름 바꾸지 않기
- MixedCaps만(언더스코어 금지), 상수도 `MaxRetries`(ALL_CAPS 금지)
- anti-stutter: `http.Client`(not `http.HTTPClient`), 단일 주요 타입 생성자는 `New()`(not `NewClient()`)
- 불리언 필드/메서드는 `is`/`has`/`can` 접두사, getter는 `Get` 생략(`user.Name()`)
- sentinel 에러는 `Err` 접두사 + 메시지에 패키지명 포함(`"roomsession: not found"`), enum zero값은 `Unknown`/`Invalid`
- 패키지명은 짧은 소문자 단수형(`roomsession`, `hub`), `util`/`helper` 금지
- 자세한 규칙·예외는 `golang-naming` 스킬과 그 references 참조

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

테스트 컨벤션은 **`golang-testing` 스킬**(samber/cc-skills-golang)을 표준으로 따른다. 프로젝트에서 특히 지키는 것:

- **테이블 기반 + named subtest**(`t.Run`), 독립 케이스는 `t.Parallel()`
- 동시성 코드는 `go test -race ./...`로 검증(Hub 등), goroutine 누수는 `goleak` 고려
- 통합 테스트는 `httptest.Server` + 실제 in-memory 조립으로 엔드투엔드 확인
- 구현 세부가 아니라 **관찰 가능한 동작/공개 계약**을 검증한다
- 그 외 패턴(fuzzing, build-tag 분리, coverage 등)은 `golang-testing` 참조

## OpenAPI (코드-퍼스트)

스펙은 **코드에서 생성**한다(`cmd/gen-openapi`). `docs/openapi.yaml`을 손으로 고치지 않는다. huma 핸들러/DTO를 바꾸면 `make openapi`로 재생성하고, `make openapi-check`가 드리프트를 막는다.

## Go 품질 스킬 (samber/cc-skills-golang)

범용 Go 품질의 표준으로 `.claude/skills/golang-*` 스킬을 설치해 두었다(이 스택에 맞게 큐레이션한 20개: `golang-naming`, `golang-code-style`, `golang-error-handling`, `golang-safety`, `golang-structs-interfaces`, `golang-concurrency`, `golang-context`, `golang-testing`, `golang-design-patterns`, `golang-modernize`, `golang-lint`, `golang-security`, `golang-observability`, `golang-documentation`, `golang-troubleshooting`, `golang-performance`, `golang-data-structures`, `golang-dependency-injection`, `golang-project-layout`, `golang-dependency-management`).

- 네이밍·에러·동시성·컨텍스트·테스트·안전·로깅 등 **언어/생태계 차원의 "어떻게"는 이 스킬들을 따른다.**
- **충돌 시 cc 스킬이 우선**한다. 이 `CLAUDE.md`는 cc 스킬이 다루지 않는 **이 프로젝트 고유 규칙**(클린 아키텍처 레이어·depguard 의존 방향, huma·`SuccessResponse[T]` 봉투, OpenAPI 코드-퍼스트, WebSocket raw 등록)만 규정한다.

## 레이어별 규칙

작업하는 레이어의 `CLAUDE.md`를 먼저 읽는다.

- [`internal/domain/CLAUDE.md`](internal/domain/CLAUDE.md) — 도메인(핵심)
- [`internal/services/CLAUDE.md`](internal/services/CLAUDE.md) — 유즈케이스
- [`internal/interfaces/CLAUDE.md`](internal/interfaces/CLAUDE.md) — HTTP/WebSocket 어댑터
- [`internal/infrastructure/CLAUDE.md`](internal/infrastructure/CLAUDE.md) — 포트 구현
- [`internal/app/CLAUDE.md`](internal/app/CLAUDE.md) — 컴포지션 루트
