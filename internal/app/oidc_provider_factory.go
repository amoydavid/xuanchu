package app

import (
	"context"

	xuanchuOIDC "git.dajee.net/dajee/xuanchu/internal/auth/oidc"
)

// newOIDCProviderFromConfig 是生产用 providerFactory，调用 internal/auth/oidc 做 discovery。
func newOIDCProviderFromConfig(issuerBaseURL, clientID, clientSecret string) (OIDCProvider, error) {
	p, err := xuanchuOIDC.NewProviderSafe(context.Background(), issuerBaseURL, clientID, clientSecret)
	if err != nil {
		return nil, err
	}
	return &realOIDCProvider{p: p}, nil
}

type realOIDCProvider struct {
	p *xuanchuOIDC.Provider
}

func (r *realOIDCProvider) AuthCodeURL(state, verifier, redirectURI string) string {
	return r.p.AuthCodeURL(state, verifier, redirectURI)
}

func (r *realOIDCProvider) Exchange(ctx context.Context, code, verifier, redirectURI string) (string, error) {
	tok, err := r.p.Exchange(ctx, code, verifier, redirectURI)
	if err != nil {
		return "", err
	}
	return tok.Subject, nil
}
