package taskfile

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeCommandShortcuts(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "command list",
			in: `version: '3'
tasks:
  msg:
    cmds:
      - echo "Message: {{.MSG}}"
`,
			want: `version: '3'
tasks:
  msg:
    cmds:
      - "echo \"Message: {{.MSG}}\""
`,
		},
		{
			name: "task shortcut",
			in: `version: '3'
tasks:
  msg: echo "Message: {{.MSG}}"
`,
			want: `version: '3'
tasks:
  msg: "echo \"Message: {{.MSG}}\""
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, []byte(test.want), normalizeCommandShortcuts([]byte(test.in)))
		})
	}
}
