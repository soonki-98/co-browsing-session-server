---
name: test-authoring
description: Use when writing Go tests for the co-browsing session server BEFORE implementation (TDD red step). Turns a spec's Acceptance Criteria into failing table-driven tests following the project's testing conventions. Output is *_test.go files only.
---

# test-authoring — 테스트 먼저 작성 (TDD RED)

구현 **전에** 실패하는 테스트를 작성한다. 스펙의 Acceptance Criteria를 테스트 케이스로 옮긴다. **`*_test.go`만 만들고 구현 코드는 건드리지 않는다.**

> 규칙의 단일 출처는 루트 [`CLAUDE.md`](../../../CLAUDE.md)와 레이어 `CLAUDE.md`. 충돌 시 그쪽 우선.

## TDD 규율

일반 TDD 사이클(RED→GREEN→REFACTOR)의 원칙은 superpowers `test-driven-development` 스킬을 따른다. 이 스킬은 그 RED 단계를 **이 프로젝트 컨벤션**으로 구체화한다.

- 한 번에 하나의 행동만 검증하는 테스트부터. 과도하게 큰 테스트를 한꺼번에 만들지 않는다.
- 테스트는 처음엔 **반드시 실패(또는 컴파일 실패)** 해야 한다 — 구현이 없으니 당연. 통과해버리면 테스트가 무의미한 것.
- Acceptance Criteria 체크리스트의 각 항목 → 최소 하나의 테스트 케이스.

## 프로젝트 테스트 컨벤션

- **테이블 기반 + `t.Run`**: 케이스를 슬라이스로 두고 `name`으로 식별, `t.Run(name, ...)`으로 격리.
- **실패 메시지는 한글로 기대를 명확히**: 예) `t.Fatalf("고객 단독 접속 시 peer는 nil이어야 한다, got %+v", peer)`.
- **네이밍은 풀네임**(루트 CLAUDE.md): 테스트 내 변수도 `roomSession`, `serialNumber`처럼 축약하지 않는다.
- **동시성 코드**는 race 테스트를 포함: 다수 goroutine으로 혼합 호출(예: Hub 100 goroutine `JoinRoom`/`LeaveRoom`), `go test -race ./...`로 검증.
- **과한 mock 금지**: 통합 테스트는 `httptest.Server` + 실제 in-memory 조립(`app.New().Engine()`)으로 엔드투엔드를 확인한다. 헬퍼(`newTestServer`, `createRoom`, `dialWS`, `readMessageType` 등) 패턴을 재사용한다.

## 레이어별 테스트 형태

- **domain**: 순수 단위 테스트. 상태 전이 규칙, 불변식, sentinel 에러를 `errors.Is`로 검증.
- **infrastructure**: 포트 구현 + 동시성(race) 테스트. 도메인 에러 규약(ErrNotFound/ErrExpired 등) 반환 확인.
- **services**: 유즈케이스 흐름 + 경계 에러 번역(도메인 에러 → 유즈케이스 에러) 검증.
- **interfaces**: huma 핸들러는 `httptest` 요청/응답(`SuccessResponse[T]` 봉투, RFC7807 에러), WebSocket은 실제 dial + 메시지 타입 검증.
- **app**: 전체 조립 기반 통합 테스트(엔드투엔드 happy path + 에러 경로).

## 실행 & 인계

- 작성 후 `go test ./<해당 패키지>/...` (또는 `go build ./...`)로 **의도대로 실패/미컴파일**임을 확인하고, 무엇이 RED인지 보고한다.
- 이 RED 테스트를 통과시키는 일은 해당 레이어 implementer(`spec-implementation`)가 맡는다 — 구현은 여기서 하지 않는다.
