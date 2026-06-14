---
name: test-authoring
description: Use when writing Go tests for the co-browsing session server BEFORE implementation (TDD red step). Turns the planning spec's Acceptance Criteria + the technical design's Test Plan into failing table-driven tests following the project's testing conventions. Output is *_test.go files only.
---

# test-authoring — 테스트 먼저 작성 (TDD RED)

구현 **전에** 실패하는 테스트를 작성한다. **기획 스펙의 인수 조건(AC-n)**을 검증 대상으로 삼고, **기술 설계도의 Test Plan/Traceability**가 각 AC를 어느 레이어의 어떤 테스트로 옮길지 알려준다. **`*_test.go`만 만들고 구현 코드는 건드리지 않는다.**

> 입력 두 개: 기획 스펙(`docs/specs/<요구사항명>/<세부요구사항>.md` — *무엇을* 검증하나, AC-n) + 기술 설계도(`docs/designs/<요구사항명>/<세부요구사항>.md` — *어느 레이어에서 어떻게* 검증하나). 설계도의 Test Plan이 비어 있으면 먼저 `tech-designer`로 보강한다.

> 이 스킬은 **TDD RED 워크플로**만 담당한다. *어떻게 좋은 Go 테스트를 쓰는가*(컨벤션·패턴)는 **`golang-testing` 스킬**(samber/cc-skills-golang)을 표준으로 따른다. 충돌 시 cc 스킬 우선. 프로젝트 고유 규칙(huma·레이어 등)은 루트 [`CLAUDE.md`](../../../CLAUDE.md).

## TDD 규율

일반 TDD 사이클(RED→GREEN→REFACTOR)의 원칙은 superpowers `test-driven-development` 스킬을 따른다. 이 스킬은 그 RED 단계를 **이 프로젝트의 스펙→테스트 흐름**으로 구체화한다.

- 한 번에 하나의 행동만 검증하는 테스트부터. 과도하게 큰 테스트를 한꺼번에 만들지 않는다.
- 테스트는 처음엔 **반드시 실패(또는 컴파일 실패)** 해야 한다 — 구현이 없으니 당연. 통과해버리면 테스트가 무의미한 것.
- 기획 AC(AC-n)의 각 항목 → 최소 하나의 테스트 케이스. 설계도 Test Plan이 지정한 레이어/테스트 종류로 배치한다.

## 테스트 컨벤션

작성 방식은 **`golang-testing` 스킬**을 따른다 — 테이블 기반 + named subtest(`t.Run`), 독립 케이스 `t.Parallel()`, race 검증, `goleak` 누수 탐지, 관찰 가능한 동작 검증(구현 세부 금지). 자세한 패턴은 그 스킬과 references 참조.

이 프로젝트에서 특히:
- **통합 테스트는 실제 조립**: `httptest.Server` + `app.New().Engine()`. 헬퍼(`newTestServer`, `createRoom`, `dialWS`, `readMessageType` 등) 패턴 재사용.
- **동시성**(Hub 등)은 다수 goroutine 혼합 호출 + `go test -race`.

## 레이어별 테스트 형태

- **domain**: 순수 단위 테스트. 상태 전이 규칙, 불변식, sentinel 에러를 `errors.Is`로 검증.
- **infrastructure**: 포트 구현 + 동시성(race) 테스트. 도메인 에러 규약(ErrNotFound/ErrExpired 등) 반환 확인.
- **services**: 유즈케이스 흐름 + 경계 에러 번역(도메인 에러 → 유즈케이스 에러) 검증.
- **interfaces**: huma 핸들러는 `httptest` 요청/응답(`SuccessResponse[T]` 봉투, RFC7807 에러), WebSocket은 실제 dial + 메시지 타입 검증.
- **app**: 전체 조립 기반 통합 테스트(엔드투엔드 happy path + 에러 경로).

## 실행 & 인계

- 작성 후 `go test ./<해당 패키지>/...` (또는 `go build ./...`)로 **의도대로 실패/미컴파일**임을 확인하고, 무엇이 RED인지 보고한다.
- 이 RED 테스트를 통과시키는 일은 해당 레이어 implementer(`spec-implementation`)가 맡는다 — 구현은 여기서 하지 않는다.
