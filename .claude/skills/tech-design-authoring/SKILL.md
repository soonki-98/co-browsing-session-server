---
name: tech-design-authoring
description: Use when turning an approved product/planning spec (docs/specs/<요구사항명>/<세부요구사항>.md) into a technical design (기술 설계도) for the co-browsing session server. Produces docs/designs/<요구사항명>/<세부요구사항>.md — the technical "how": layer placement, data structures, ports, error/concurrency strategy, file locations, and a test plan tracing back to the planning AC. Output is a design document, never implementation code.
---

# tech-design-authoring — 기술 설계도 작성

승인된 **기획 요구사항**(`docs/specs/<요구사항명>/<세부요구사항>.md`)을 받아, 그걸 *어떻게* 구현할지 결정한 **기술 설계도**를 작성한다. clean architecture 레이어 배치, 자료구조, 포트, 에러·동시성 전략, 파일 위치, 그리고 기획 AC로 되돌아가는 테스트 계획을 담는다.

> **출력은 설계 문서이고, 구현 코드를 만들지 않는다.** 설계도는 구현(`spec-implementation`)과 테스트(`test-authoring`)의 단일 입력이 된다.
>
> 프로젝트 규칙의 단일 출처는 루트 [`CLAUDE.md`](../../../CLAUDE.md)와 각 레이어 `CLAUDE.md`다. **Go 코드 품질·설계 판단(네이밍·에러·동시성·컨텍스트·구조체/인터페이스·디자인 패턴)은 `golang-*` 스킬**(samber/cc-skills-golang)을 표준으로 따른다 — 충돌 시 cc 스킬 우선.

## 파일 위치 / 네이밍 (기획과 1:1 미러)

```
기획:    docs/specs/<요구사항명>/<세부요구사항>.md
기술설계: docs/designs/<요구사항명>/<세부요구사항>.md   ← 같은 경로를 미러
```

설계도는 대응하는 기획 스펙과 **같은 `<요구사항명>/<세부요구사항>` 경로**를 쓴다(접두 디렉터리만 `specs`→`designs`). 그래야 둘의 대응이 경로만으로 자명하다.

## 먼저 (입력 읽기)

1. 대상 **기획 스펙**을 끝까지 읽는다 — FR-n, 비즈니스 룰, AC-n 번호를 파악한다.
2. 루트 `CLAUDE.md`의 "새 코드를 어느 레이어에 둘지"·의존 규칙, 그리고 건드릴 레이어의 `CLAUDE.md`를 읽는다.
3. 기존 코드(`internal/...`)와 기존 설계도(`docs/designs/`)를 grep해, 재사용할 타입/포트와 일관된 패턴을 확인한다.

## 템플릿

```markdown
# <기능명> — 기술 설계도

> 기획: [docs/specs/<요구사항명>/<세부요구사항>.md](../../specs/<요구사항명>/<세부요구사항>.md)

## Overview
이 기획을 어떻게 구현하는지 한 문단. 핵심 기술 접근과 핵심 결정.

## Implementation Order
ASCII 트리로 선행/후행 의존성. 의존 순서(domain → infrastructure → services → interfaces → app)와 일치. "← 지금 여기" 표시.

## Layer Mapping
각 FR/유닛을 clean architecture 레이어에 배치. 의존 방향(안쪽 지향, depguard)을 위반하지 않음을 확인.
| 기획 FR | 레이어 | 담당 유닛 |

## Dependencies
실제 import 블록(파일 경로 주석). 신규 외부 패키지(`go get ...`). 변경 없는 패키지 명시.

## Data Structures
레이어별 Go 코드 블록(엔티티·값 객체·구조체). 파일 경로 주석 필수.

## Interfaces / Contracts
도메인 포트 인터페이스, 도메인 행동 메서드 시그니처, 상수. 포트는 소비자 쪽 필요 최소로(accept interfaces).

## Behavior
상태 머신(ASCII), TTL/만료 규칙, sentinel 에러(`var ErrXxx = ...`) 정의와 레이어별 번역, 동시성 전략(뮤텍스/채널, race 안전성).

## File Locations
| 작업 | 파일 |  ← 신규 생성 / 수정 / 삭제 를 표로

## Test Plan (기술 인수 기준)
기획 AC(AC-n)를 레이어별 테스트로 매핑. domain 단위 / infrastructure race / services 유즈케이스 / interfaces 통합. 동시성은 `go test -race` 명시. 이 계획이 `test-authoring`에서 1:1 테스트로 옮겨진다.

## Traceability
| 기획 FR/AC | 구현 위치(레이어·파일) | 검증 테스트 |
모든 기획 FR/AC가 빠짐없이 한 줄씩 매핑돼야 한다.
```

## 작성 규칙

- **언어**: 영문 섹션 제목 + 한글 서술. 코드 블록은 Go.
- **레이어 매핑 정확성**: Data Structures/Contracts를 정확한 레이어에 배치한다. 안쪽을 향하지 않는 의존이 설계에 들어가지 않게 — depguard가 잡기 전에 설계에서 막는다.
- **Implementation Order = 의존 순서**(domain → infrastructure → services → interfaces → app).
- **Go 품질 결정은 cc 스킬 위임**: 네이밍은 `golang-naming`(관용), 에러는 `golang-error-handling`(sentinel + `%w`), 동시성은 `golang-concurrency`, 구조체/포트는 `golang-structs-interfaces`. 이 문서는 *무엇을 어디에 둘지*를 정하고, *어떻게 잘 쓰는지*는 그 스킬을 참조한다.
- **추적성 필수**: 모든 FR/AC가 Traceability 표에 매핑돼야 한다. 매핑되지 않는 기획 항목이 있으면 설계가 불완전한 것.
- **기획을 바꾸지 않는다**: 기획이 모호하거나 구현 불가능하면, 임의로 정하지 말고 멈춰서 기획(spec-writer)으로 되돌려 질문한다. 기술 설계도는 기획을 *해석*할 뿐 *변경*하지 않는다.

## 자기 점검 (작성 후)

- [ ] 대응 기획 스펙과 같은 `<요구사항명>/<세부요구사항>` 경로(`docs/designs/` 하위)에 있다
- [ ] 모든 기획 FR/AC가 Traceability 표에 매핑됐다(누락 0)
- [ ] Implementation Order가 의존 순서와 모순 없고, Layer Mapping이 depguard 방향을 위반하지 않는다
- [ ] File Locations 경로가 실제 패키지 구조와 일치한다
- [ ] Test Plan의 각 항목이 테스트로 옮길 만큼 구체적이고, 동시성 항목은 `go test -race`를 명시한다
- [ ] 구현 코드를 쓰지 않았다 — 산출물은 설계 문서 한 편뿐이다
