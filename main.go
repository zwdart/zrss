// zrss is a local-first RSS reader for the desktop, drawn with MyGo's
// native UI: no WebView, no account, and all of its data in one SQLite
// file on this machine.
package main

import (
	"log"

	"xin.dart.zrss/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
