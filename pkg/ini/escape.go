package ini

import "strings"

// Allowed escaping characters by gopkg.in/gcfg.v1 - the lib kubernetes uses
var escaper = strings.NewReplacer(
	`\`, `\\`,
	`"`, `\"`,
)

// INIEscape escapes values in ini files correctly according to gopkg.in/gcfg.v1 - the lib kubernetes uses
func INIEscape(s string) string {
	return `"` + escaper.Replace(s) + `"`
}
