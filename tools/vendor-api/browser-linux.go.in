// FractalCR serial-console adapter. The client receives a link; the appliance
// never launches a desktop browser or a host command to open it.
package browser

import (
	"fmt"
	"net/url"
)

func openBrowser(link string) error {
	parsed, err := url.Parse(link)
	if err != nil || parsed.Scheme == "" {
		return fmt.Errorf("Invalid browser link")
	}
	_, err = fmt.Fprintln(Stdout, link)
	return err
}
