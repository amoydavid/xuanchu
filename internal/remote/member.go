package remote

import (
	"context"
	"net/url"

	"git.dajee.net/dajee/xuanchu/internal/app"
)

type memberDTO struct {
	UserID     string  `json:"user_id"`
	Name       string  `json:"name"`
	Email      *string `json:"email,omitempty"`
	Role       string  `json:"role"`
	JoinedAt   int64   `json:"joined_at"`
	ModifiedAt int64   `json:"modified_at"`
}

type AddMemberInput struct {
	User string `json:"user"`
	Role string `json:"role"`
}

type changeMemberRoleInput struct {
	Role string `json:"role"`
}

func (c *Client) ListMembers(ctx context.Context, workspace string) ([]app.MemberView, error) {
	path := "/api/v1/workspaces/" + url.PathEscape(workspace) + "/members"
	var envelope apiEnvelope[[]memberDTO]
	if err := c.get(ctx, path, nil, &envelope); err != nil {
		return nil, err
	}
	out := make([]app.MemberView, 0, len(envelope.Data))
	for _, row := range envelope.Data {
		out = append(out, memberDTOToView(row))
	}
	return out, nil
}

func (c *Client) AddMember(ctx context.Context, workspace string, input AddMemberInput) error {
	path := "/api/v1/workspaces/" + url.PathEscape(workspace) + "/members"
	return c.post(ctx, path, input, nil)
}

func (c *Client) ChangeMemberRole(ctx context.Context, workspace, user, role string) error {
	path := "/api/v1/workspaces/" + url.PathEscape(workspace) + "/members/" + url.PathEscape(user)
	return c.patch(ctx, path, changeMemberRoleInput{Role: role}, nil)
}

func memberDTOToView(row memberDTO) app.MemberView {
	return app.MemberView{
		UserID:     row.UserID,
		Name:       row.Name,
		Email:      row.Email,
		Role:       app.Role(row.Role),
		JoinedAt:   row.JoinedAt,
		ModifiedAt: row.ModifiedAt,
	}
}
