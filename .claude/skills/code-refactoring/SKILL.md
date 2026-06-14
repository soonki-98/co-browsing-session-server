---
name: code-refactoring
description: Use when improving existing Go code in the co-browsing session server without changing behavior — extracting functions, moving logic to the correct layer, renaming to full-word names, introducing ports, removing duplication. Keeps tests green and respects depguard layer rules.
---

# code-refactoring — 동작 보존 리팩터링

겉보기 동작을 바꾸지 않고 코드를 개선한다. **테스트가 안전망**이다.

> 규칙의 단일 출처는 루트 [`CLAUDE.md`](../../../CLAUDE.md)와 레이어 `CLAUDE.md`. 충돌 시 그쪽 우선.

## 안전 프로토콜 (반드시)

1. **시작 전 green**: `go test ./...`(+필요 시 `-race`)가 통과하는 상태에서 시작한다. 실패 중이면 먼저 그걸 해결하거나 멈춘다.
2. **작은 단계로**: 한 번에 하나의 리팩터링. 단계마다 테스트를 돌린다.
3. **동작 불변**: 공개 동작/시그니처가 바뀌면 그건 리팩터링이 아니다. 테스트를 고쳐야 한다면 멈추고 의도를 확인한다.
4. **끝나고 green + lint**: `go test ./...`와 `golangci-lint run` 통과. 엔드포인트 영향 시 `make openapi-check`.
- 리팩터링 중 버그를 발견하면, 별도로 분리하고 superpowers `systematic-debugging`으로 다룬다(리팩터링과 버그픽스를 섞지 않는다).

## 카탈로그 (이 프로젝트에서 자주)

- **레이어 위반 교정**: 잘못된 레이어에 있는 로직을 올바른 레이어로 이동(예: 핸들러의 비즈니스 규칙 → services/domain). depguard가 가리키는 의존 위반을 안쪽-지향으로 바로잡는다.
- **풀네임 리네임**: 축약 receiver/변수(`h`, `repo`, `gen`)를 풀네임(`handler`, `roomSessionRepository`, `generator`)으로. Go 관용보다 프로젝트 규칙 우선.
- **포트 추출**: 구체 의존을 도메인 포트(인터페이스)로 추출해 의존 역전. 인터페이스는 소비자 쪽 필요 최소로.
- **에러 처리 정리**: 맥락 없는 에러를 `fmt.Errorf("...: %w", err)`로 래핑, 분기 에러를 sentinel + `errors.Is`로.
- **로깅 정리**: 산발적 로그를 경계 단일 지점으로, `log/slog` 구조적 로깅 지향. 안쪽 레이어의 이중 로깅 제거.
- **중복 제거 / 함수 추출 / 거대 파일 분할**: 한 파일·타입이 한 책임만 갖도록.

## 범위 규율

- 요청된 목표에 집중한다. 무관한 대규모 리팩터링을 끼워 넣지 않는다.
- 레이어를 넘나드는 변경은 의존 방향(안쪽 우선)을 유지하며 진행한다.
