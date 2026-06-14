---
name: spec-implementation
description: Use when implementing a written docs/specs spec into Go code for the co-browsing session server. Drives TDD (make failing tests pass), respects clean-architecture layer rules, and runs the project's test/lint/openapi gates. Shared by the orchestrator and the per-layer implementer agents.
---

# spec-implementation — 스펙 기반 구현 (TDD)

작성된 스펙을 코드로 옮긴다. **테스트 먼저(RED) → 최소 구현(GREEN) → 리팩터링** 순서를 지킨다.

> 프로젝트 고유 규칙(레이어·huma·OpenAPI)은 루트 [`CLAUDE.md`](../../../CLAUDE.md)와 작업 레이어의 `CLAUDE.md`. 시작 전 해당 레이어 `CLAUDE.md`를 읽는다. **Go 코드 품질(네이밍·에러·동시성·컨텍스트·테스트·안전)은 `golang-*` 스킬**(samber/cc-skills-golang)을 표준으로 따른다 — 충돌 시 cc 스킬 우선.

## 절차

1. **스펙 파싱**: Overview/Contracts/Behavior/Acceptance Criteria를 읽고, 만들 유닛과 그 레이어를 식별한다.
2. **의존 순서로 분해**: `domain → infrastructure → services → interfaces → app`. 안쪽이 먼저 GREEN이어야 바깥이 주입받을 수 있다.
3. **유닛마다 TDD 사이클**:
   - **RED**: 실패 테스트가 있어야 한다. 없으면 `test-authoring`(또는 `test-writer` agent)으로 먼저 확보한다. **실패 테스트 없이 구현을 시작하지 않는다.**
   - **GREEN**: 그 테스트를 통과시키는 **최소** 구현. 스펙에 없는 기능을 추가하지 않는다(YAGNI).
   - **REFACTOR**: 테스트 green을 유지하며 정리. 필요 시 `code-refactoring`.
4. 일반 TDD 규율은 superpowers `test-driven-development`를 따른다.

## 레이어 규칙 (요약 — 상세는 각 레이어 CLAUDE.md)

- **domain**: 외부 레이어/프레임워크 import 금지. 엔티티·값 객체·포트·sentinel 에러만.
- **infrastructure**: 도메인 포트 구현. 도메인만 의존. 공유 상태는 mutex 보호.
- **services**: 유즈케이스. 포트(인터페이스)만 주입받음. 인프라 구현 직접 참조 금지. 도메인 에러를 유즈케이스 경계 에러로 번역.
- **interfaces**: huma 핸들러는 서브패키지(`Handler`+`Register(api)`+`dto.go`), 성공은 `SuccessResponse[T]`, 에러는 huma `ErrorModel`. WebSocket은 raw gin 등록. 비즈니스 로직 금지.
- **app**: 컴포지션 루트. 구체 타입 생성·주입·라우팅 조립만.

## 게이트 (각 유닛/마무리)

- `go test ./...` 그리고 동시성 관련은 `go test -race ./...` 통과
- `golangci-lint run` 통과 — **depguard 레이어 의존 위반은 여기서 잡힌다.** import 추가 전 "안쪽을 향하는가" 확인.
- 엔드포인트/DTO를 바꿨으면 `make openapi`로 `docs/openapi.yaml` 재생성(수기 편집 금지), `make openapi-check`로 드리프트 확인.
- Acceptance Criteria 체크리스트를 모두 충족했는지 대조.

## 메인 스레드 오케스트레이션 (참고)

여러 레이어가 걸친 스펙은 메인 스레드에서 유닛별로 `test-writer` → 해당 `<layer>-implementer` 순서로 디스패치한다(subagent는 다시 subagent를 못 띄우므로 디스패치는 메인 스레드가 수행). 레이어 implementer는 **자기 레이어 디렉터리만** 수정한다.
