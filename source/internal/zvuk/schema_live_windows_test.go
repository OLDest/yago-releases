package zvuk

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"yago/internal/store"
)

func TestInspectPlaybackSchema(t *testing.T) {
	state := os.Getenv("YAGO_ZVUK_LIVE_STATE")
	if state == "" {
		t.Skip("explicit read-only schema probe")
	}
	device, e := os.ReadFile(filepath.Join(state, "device-id"))
	if e != nil {
		t.Fatal("device unavailable")
	}
	token, e := store.Load(string(device) + ":zvuk")
	if e != nil {
		t.Fatal("saved account unavailable")
	}
	c := New(token.Access)
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var result struct {
		Schema struct {
			MutationType struct{ Name string }
			QueryType    struct{ Name string }
		} `json:"__schema"`
	}
	if e := c.gql(ctx, `query{__schema{mutationType{name} queryType{name}}}`, nil, &result); e != nil {
		t.Fatal("Schema introspection unavailable:", e)
	}
	for _, root := range []string{result.Schema.MutationType.Name, result.Schema.QueryType.Name} {
		var data struct {
			Type struct {
				Fields []struct {
					Name string
					Args []struct {
						Name string
						Type struct {
							Kind, Name string
							OfType     *struct{ Kind, Name string }
						}
					}
				}
			} `json:"__type"`
		}
		if e := c.gql(ctx, `query($name:String!){__type(name:$name){fields{name args{name type{kind name ofType{kind name}}}}}}`, map[string]any{"name": root}, &data); e != nil {
			t.Fatal(e)
		}
		for _, field := range data.Type.Fields {
			n := strings.ToLower(field.Name)
			if strings.Contains(n, "listen") || strings.Contains(n, "play") || strings.Contains(n, "history") || strings.Contains(n, "event") || strings.Contains(n, "progress") {
				t.Logf("%s.%s args=%+v", root, field.Name, field.Args)
			}
		}
	}
}
