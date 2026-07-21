package compose

import "testing"

func TestParseNameConflicts(t *testing.T) {
	msg := `Error response from daemon: Conflict. The container name "/postgres" is already in use by container "75d33bfcfefc".
Container keycloak Creating
Error response from daemon: Conflict. The container name "/postgres" is already in use`
	names := ParseNameConflicts(msg)
	if len(names) != 1 || names[0] != "postgres" {
		t.Fatalf("got %v", names)
	}
}

func TestCanSafelyRemove(t *testing.T) {
	cases := []struct {
		name    string
		c       *ContainerInfo
		project string
		want    bool
	}{
		{
			name: "our project exited",
			c:    &ContainerInfo{Project: "camunda-lab", State: "exited"},
			want: true,
		},
		{
			name: "exited camunda image",
			c:    &ContainerInfo{Project: "", State: "exited", Image: "camunda/zeebe:8.9"},
			want: true,
		},
		{
			name: "running foreign",
			c:    &ContainerInfo{Project: "other-app", State: "running", Image: "postgres:16"},
			want: false,
		},
		{
			name: "running ours",
			c:    &ContainerInfo{Project: "camunda-lab", State: "running"},
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanSafelyRemove(tc.c, "camunda-lab"); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestIsCamundaRelated(t *testing.T) {
	if !IsCamundaRelated("camunda/operate:8.9", "") {
		t.Fatal("expected camunda image")
	}
	if IsCamundaRelated("nginx:latest", "myblog") {
		t.Fatal("unexpected match")
	}
}
