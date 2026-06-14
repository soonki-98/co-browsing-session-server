---
name: spec-authoring
description: Use when writing a new requirements/implementation spec under docs/specs/ for the co-browsing session server. Produces a docs/specs/NN-*.md document in the project's 8-section template. Output is a spec document, never code.
---

# spec-authoring — 요구사항 스펙 작성

이 프로젝트의 구현 스펙(`docs/specs/NN-*.md`)을 정해진 템플릿으로 작성한다. **출력은 문서이고, 코드를 만들지 않는다.**

> 프로젝트 규칙의 단일 출처는 루트 [`CLAUDE.md`](../../../CLAUDE.md)와 각 레이어 `CLAUDE.md`다. 규칙이 충돌하면 항상 그쪽을 따른다.

## 먼저 (의도 도출)

무엇을 만들지 아직 모호하면, 일반적인 아이디어→설계 정리는 superpowers `brainstorming` 스킬에 위임한다. 이 스킬은 **합의된 요구사항을 프로젝트 스펙 포맷으로 옮기는 단계**를 담당한다.

기존 스펙을 먼저 읽어 번호·스타일·의존 관계를 맞춘다:
- `docs/specs/` 목록을 확인하고 다음 번호(`NN`)를 정한다.
- `docs/specs/01-session-store.md`를 레퍼런스 포맷으로 삼는다.
- `docs/requirements.md`로 전체 맥락(세션 흐름, WS 프로토콜)을 확인한다.

## 템플릿 (8섹션, 순서 고정)

```markdown
# NN. <제목> Spec

## Overview
2~3문장으로 목표. 전체 N단계 중 몇 번째인지, 왜 이 묶음인지.

## Implementation Order
ASCII 트리로 선행/후행 의존성. "← 지금 여기" 표시. 선행/후행 의존성 불릿.

## Dependencies
실제 import 블록(파일 경로 주석 포함). 신규 외부 패키지(`go get ...`). 변경 없는 패키지 명시.

## Data Structures
레이어별 Go 코드 블록(엔티티·값 객체·구조체). 파일 경로 주석 필수.

## Interfaces / Contracts
도메인 포트 인터페이스, 도메인 행동 메서드 시그니처, 상수.

## Behavior
상태 머신(ASCII), TTL/만료 규칙, 에러 타입(sentinel) 정의, 동시성 전략.

## File Locations
| 작업 | 파일 |  ← 신규 생성 / 수정 / 삭제 를 표로

## Acceptance Criteria
레이어별 그룹. `- [ ]` 체크리스트. 입력→기대결과 형태로 구체적으로.
동시성 항목은 반드시 `go test -race` 명시.
```

## 작성 규칙

- **언어**: 영문 섹션 제목 + 한글 서술(레퍼런스 스펙과 동일). 코드 블록은 Go.
- **레이어 매핑**: Data Structures/Contracts를 clean architecture 레이어에 정확히 배치한다(domain/services/interfaces/infrastructure). 의존 방향은 루트 `CLAUDE.md`의 표를 따른다 — 안쪽을 향하지 않는 의존이 스펙에 들어가지 않게 한다.
- **Implementation Order**는 항상 의존 순서(domain → infrastructure → services → interfaces → app)와 일치해야 한다.
- **Acceptance Criteria는 테스트 가능하게** 쓴다. 이 체크리스트가 뒤에 `test-authoring`에서 테스트 케이스로 1:1 매핑되므로, 각 항목은 "이 입력에 이 결과/에러"로 검증 가능해야 한다.
- 에러는 sentinel(`var ErrXxx = errors.New(...)`)로 명세하고, 어느 레이어가 어떤 에러를 반환/번역하는지 적는다.

## 자기 점검 (작성 후)

- [ ] 8섹션 모두 있고 빈 자리(TBD/placeholder) 없음
- [ ] Implementation Order가 의존 순서와 모순 없음
- [ ] File Locations의 경로가 실제 패키지 구조와 일치
- [ ] 모든 Acceptance Criteria가 테스트로 옮길 수 있을 만큼 구체적
- [ ] 레이어 의존 방향이 depguard 규칙을 위반하지 않음
