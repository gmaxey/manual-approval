package manual_approval

import (
	"context"
	"net/http"
)

type HttpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type StdOut interface {
	Printf(format string, a ...any)
	Println(a ...any)
}

type Config struct {
	context.Context
	Client HttpClient
	Output StdOut

	// Handler field allows you to handler.
	Handler string `json:"handler,omitempty"`
}

type CreateManualApprovalResponse struct {
	Approvers []Approvers `json:"approvers"`
}

type Approvers struct {
	UserName string `json:"userName"`
	UserId   string `json:"userId"`
	Email    string `json:"email"`
}
