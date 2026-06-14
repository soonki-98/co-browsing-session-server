---
name: code-verification
description: Use to verify the co-browsing session server end-to-end and iteratively fix issues until clean — designed to run inside an autonomous loop (/loop code-verification, or /goal where that command exists). Defines an explicit goal and completion criteria so the loop knows when to stop.
---

# code-verification — 무한 검증 루프

코드 전체를 검증하고, 발견된 문제를 고치고, 다시 검증한다. **완료조건을 만족할 때까지 반복**한다. 자율 루프(`/loop code-verification`, 환경에 `/goal`이 있으면 `/goal`)가 이 스킬을 반복 구동한다.

> 규칙의 단일 출처는 루트 [`CLAUDE.md`](../../../CLAUDE.md)와 레이어 `CLAUDE.md`. 충돌 시 그쪽 우선.

## GOAL

저장소가 **빌드·테스트·린트·스펙 일관성** 모든 게이트를 통과하고, 추가 셀프리뷰에서 더 이상 새 이슈가 나오지 않는 상태.

## 완료조건 (모두 충족 + 연속 2라운드 새 이슈 0이면 정지)

- [ ] `go build ./...` 성공
- [ ] `go test ./...` 통과
- [ ] `go test -race ./...` 통과
- [ ] `golangci-lint run` clean (depguard 레이어 위반 0 포함)
- [ ] `make openapi-check` clean (OpenAPI 스펙 드리프트 없음)
- [ ] 셀프리뷰가 **연속 2라운드** 새 이슈 0

## 루프 (한 라운드)

1. **검증 파이프라인 실행** — 위 게이트를 순서대로 돌리고 출력을 그대로 수집한다(요약하지 말고 실패 메시지 보존).
2. **발견 수집** — 실패/경고/리뷰 지적을 목록화. 각 항목에 원인 가설.
3. **수정** — 한 번에 하나씩 고친다. 동작 변경 없는 정리는 `code-refactoring`, 버그는 superpowers `systematic-debugging`(+`golang-troubleshooting`), 누락 구현은 `spec-implementation`(테스트 먼저)으로. lint/품질 지적은 `golang-lint`·`golang-*` 스킬 기준으로 해소.
4. **재검증** — 게이트 재실행. 고친 게 회귀를 만들지 않았는지 확인.
5. **셀프리뷰** — "놓친 게 있나: 검증 안 된 경로, 빠진 테스트, 레이어 경계, 미세 경합, 프로덕션 코드의 설명형 주석(=리팩터링 신호; 예외 테스트·DTO/스펙)?" 새 이슈가 있으면 다음 라운드로, 없으면 카운트 증가.
6. **정지 판단** — 완료조건 충족 + 연속 2라운드 새 이슈 0 → 정지하고 최종 보고. 아니면 다음 라운드.

마무리 보고는 superpowers `verification-before-completion` 규율을 따른다 — **증거(명령 출력) 먼저, 주장은 그다음.** 통과를 증명 없이 단언하지 않는다.

## 안전 한계

- 무한 루프지만 **수렴해야** 한다. 같은 이슈가 3라운드 반복되면 정지하고 사람에게 에스컬레이션한다(접근을 바꿔야 한다는 신호).
- 테스트를 통과시키려고 테스트를 약화시키지 않는다. 사양이 틀렸다면 멈추고 보고한다.

## 실행

- 자율 루프: `/loop code-verification`(self-paced) 또는 `/goal`(사용자 환경에 있으면). 어느 쪽이든 이 스킬 한 라운드를 반복하며, 완료조건에서 스스로 멈춘다.
