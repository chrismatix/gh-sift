package ghtest

import "testing"

func TestFakeAnswersGraphQLAndREST(t *testing.T) {
	server, client := New(t)
	server.GraphQL("viewer", Reply(200, map[string]any{"viewer": map[string]any{"login": "octocat"}}))
	login, err := client.ViewerLogin()
	if err != nil || login != "octocat" {
		t.Fatalf("login = %q, %v", login, err)
	}
	if len(server.Find("viewer")) != 1 {
		t.Errorf("requests = %+v", server.Requests)
	}
	if _, err := client.Attributes("owner/repo", "abc"); err != nil {
		t.Errorf("a missing .gitattributes is not an error: %v", err)
	}
}
