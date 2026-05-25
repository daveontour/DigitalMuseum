package handler

import "testing"

func TestBootstrapAdminOK(t *testing.T) {
	h := &AdminUsersHandler{}
	h.WithBootstrapAdminCredentials("Admin@Example.com", "MySecretPass")

	if !h.bootstrapAdminOK("admin@example.com", "mysecretpass") {
		t.Fatal("expected matching credentials")
	}
	if !h.bootstrapAdminOK(" Admin@Example.com ", "MySecretPass") {
		t.Fatal("expected trim/case-insensitive email and lowercase password match")
	}
	if h.bootstrapAdminOK("admin@example.com", "wrong") {
		t.Fatal("expected wrong password to fail")
	}
	if h.bootstrapAdminOK("other@example.com", "MySecretPass") {
		t.Fatal("expected wrong email to fail")
	}
}
