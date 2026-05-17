# Session Server 요구사항

**작성일:** 2026-05-17  
**상태:** 승인됨  
**원본 문서:** `../docs/superpowers/specs/2026-05-17-co-browsing-requirements-design.md`

---

## 개요

고객 지원(CS) 시나리오에서 상담사가 고객의 브라우저 탭을 실시간으로 보고 원격 제어할 수 있는 co-browsing 플랫폼의 세션 서버.

### 핵심 설계 결정

| 항목 | 결정 |
|------|------|
| 사용 시나리오 | 고객 지원 (CS) |
| MVP 제어 수준 | 상담사가 고객 화면 완전 원격 제어 |
| 세션 시작 방식 | 고객이 크롬 익스텐션에서 시리얼 번호 생성 → 상담사가 웹 콘솔에 입력 |
| 화면 공유 방식 | WebRTC 스크린 캡처 스트리밍 + TURN 서버 폴백 |
| 인증 | MVP에서는 없음 |

---

## 1. 전체 시스템 아키텍처 (서버 관점)

### 구성요소

```
[고객] Chrome Extension
  - 시리얼 번호 생성 요청
  - 현재 탭 화면 캡처 (chrome.tabCapture)
  - WebRTC 스트림 송출
  - 상담사의 제어 이벤트 수신 → 탭에 주입

[상담사] Web Console
  - 시리얼 번호 입력
  - WebRTC 스트림 수신 → 화면 표시
  - 마우스/키보드 이벤트 캡처 → 서버로 전송

[서버] Co-browsing Session Server (Go)
  - 시리얼 번호 발급/관리
  - WebSocket 시그널링 중계 (WebRTC offer/answer/ICE)
  - 제어 이벤트 중계 (상담사 → 고객)
  - TURN 서버 자격증명 발급
```

### 세션 흐름

```
1. 고객이 익스텐션 사이드패널에서 "세션 시작" 클릭
2. 서버 POST /serial_number 호출 → 6자리 코드 발급
3. 고객이 시리얼 번호를 상담사에게 전달 (구두/복사)
4. 상담사가 웹 콘솔에 시리얼 번호 입력
5. 양쪽 모두 WebSocket으로 서버에 접속 (room_id = 시리얼 번호)
6. 서버가 WebRTC 시그널링 중계 → P2P 연결 수립
7. P2P 실패 시 TURN 서버 경유 자동 폴백
8. 고객 탭 화면이 상담사에게 스트리밍
9. 상담사 클릭/스크롤 → 서버 중계 → 고객 탭에 이벤트 주입
```

---

## 2. Session Server 요구사항

### API 엔드포인트

| Method | Path | 설명 |
|--------|------|------|
| `POST` | `/serial_number` | 6자리 시리얼 번호 발급 + 세션 등록 |
| `GET` | `/ws?serial=XXXXXX` | WebSocket 연결 (시그널링 + 이벤트 중계) |
| `GET` | `/turn-credentials` | TURN 서버 임시 자격증명 발급 |
| `GET` | `/ping` | 헬스체크 |

### 기능 요구사항

#### 세션 관리
- 시리얼 번호 발급 시 세션 메모리에 등록
- 세션 상태 추적: `waiting` → `active` → `ended`
- TTL: 고객 연결 후 10분 이내 상담사 미접속 시 자동 만료

#### WebSocket 시그널링
- 연결 시 role 구분: `customer` / `agent`
- WebRTC 시그널링 메시지 중계: `offer` / `answer` / `ice-candidate`
- 양쪽 모두 연결되면 시그널링 시작 (`peer-joined` 이벤트 발송)

#### 제어 이벤트 중계
- 상담사 제어 이벤트를 고객에게 그대로 중계
- 이벤트에 타임스탬프 포함

#### TURN 자격증명
- 임시 자격증명 반환 (유효시간 1시간)

### 비기능 요구사항
- 세션 데이터 메모리 저장 (MVP, DB 불필요)
- CORS: 웹 콘솔 도메인 허용

---

## 3. WebSocket 메시지 프로토콜

모든 메시지는 JSON 형식: `{ "type": "...", "payload": { ... } }`

### 클라이언트 → 서버

| type | 발신자 | payload |
|------|--------|---------|
| `join` | 고객/상담사 | `{ serial, role: "customer" \| "agent" }` |
| `offer` | 고객 | `{ sdp }` (WebRTC SDP offer) |
| `answer` | 상담사 | `{ sdp }` (WebRTC SDP answer) |
| `ice-candidate` | 양쪽 | `{ candidate }` |
| `control-event` | 상담사 | `{ type: "click" \| "scroll" \| "keydown", x?, y?, key?, deltaY?, timestamp }` |
| `leave` | 양쪽 | `{}` |

### 서버 → 클라이언트

| type | 수신자 | payload |
|------|--------|---------|
| `peer-joined` | 고객 | `{}` (상담사 접속 알림, 시그널링 시작 트리거) |
| `offer` | 상담사 | `{ sdp }` |
| `answer` | 고객 | `{ sdp }` |
| `ice-candidate` | 양쪽 | `{ candidate }` |
| `control-event` | 고객 | `{ type, x?, y?, key?, deltaY?, timestamp }` |
| `peer-left` | 양쪽 | `{}` |
| `error` | 양쪽 | `{ code, message }` |

---

## 4. 미래 고도화 (MVP 이후)

- 고객 승인 기반 제어권 전환 (보기 전용 ↔ 원격 제어)
- 상담사 인증 (로그인)
- 세션 히스토리 저장 (DB 연동)
- TURN 서버 자체 운영
