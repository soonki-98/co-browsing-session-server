# interfaces — 작업 규칙

> 개념·역할은 [`README.md`](README.md) 참조. 이 문서는 작업 시 지킬 규칙이다. 공통 규칙은 [루트 `CLAUDE.md`](../../CLAUDE.md).

interfaces는 **외부 프로토콜(HTTP, WebSocket)을 유즈케이스 호출로 변환하는 어댑터**다. 외부 세계와 애플리케이션 사이의 경계 — 파싱/검증/직렬화/프로토콜 에러 변환만 책임지고, 비즈니스 규칙은 절대 두지 않는다.

## 의존 규칙

✅ `services`(유즈케이스)와 `domain`(타입·포트)에 의존한다
🚫 `infrastructure` 구현 직접 의존 금지 — 인프라 자원이 필요해도 서비스/포트를 경유한다
🚫 `app` 의존 금지
🚫 비즈니스 규칙·상태 전이 로직을 핸들러에 두지 않는다 (그건 services/domain의 몫)

## 구조

- HTTP 엔드포인트는 `http/` 아래, WebSocket은 `ws/` 아래로 트랜스포트별로 분리한다
- 엔드포인트는 **서브패키지 단위**로 둔다(`http/room`, `http/ping`). 각 서브패키지는 `Handler` 구조체 + `Register(...)` + `dto.go` 를 가진다

## HTTP 패턴 (huma v2, 코드-퍼스트)

- `Handler`는 의존(서비스)을 생성자로 주입받고, `Register(api huma.API)`에서 `huma.Register`로 오퍼레이션을 등록한다
- 입출력은 **DTO 구조체**로 표현한다. 성공 응답은 공통 `SuccessResponse[T]` 봉투(`{"data": ...}`)를 쓰고, 에러는 huma `ErrorModel`(RFC 7807)
- 핸들러는 입력을 검증/매핑 → 서비스 호출 → 결과를 DTO로 직렬화한다. 에러는 `huma.ErrorXxx`로 트랜스포트 에러로 변환
- 스펙은 코드에서 나온다. DTO/오퍼레이션을 바꾸면 `make openapi`로 재생성한다 (`docs/openapi.yaml` 수기 편집 금지)

## WebSocket 패턴

- WS 업그레이드는 raw `http.ResponseWriter`가 필요해 huma 타입드 핸들러를 통과할 수 없다 → **gin 엔진에 직접 라우트 등록**한다(`Register(engine *gin.Engine)`)
- 핸들러는 트랜스포트 책임만 진다: 업그레이드, 바이트 ↔ 메시지 DTO 직렬화, read/write 펌프. 연결 수립/해제의 유즈케이스 흐름은 서비스(코디네이터)에 위임한다
- 유즈케이스 경계 에러를 프로토콜 응답으로 번역한다(`errors.Is`로 분기 → HTTP 상태 코드 또는 WS 에러 메시지)
- 동시성: 클라이언트별 read/write를 분리하고, 송신 채널은 버퍼드 + 비차단으로 다뤄 슬로우 클라이언트가 전체를 막지 않게 한다
