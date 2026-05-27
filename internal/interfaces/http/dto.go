package http

import "github.com/danielgtaylor/huma/v2"

// SuccessResponse는 모든 성공 응답의 공통 베이스다. T에 각 엔드포인트의 페이로드 타입을 주입해
// 구상 응답 타입을 만든다. JSON 상으로는 항상 `{"data": ...}` 봉투를 갖는다.
type SuccessResponse[T any] struct {
	Body struct {
		Data T `json:"data"`
	}
}

// ErrorResponse는 모든 에러 응답의 공통 베이스다. huma의 RFC 7807 ErrorModel을 그대로 사용한다.
type ErrorResponse = huma.ErrorModel
