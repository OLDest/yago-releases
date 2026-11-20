package updates

import "strings"

func windowsPathEqual(a, b string) bool { return strings.EqualFold(a, b) }
