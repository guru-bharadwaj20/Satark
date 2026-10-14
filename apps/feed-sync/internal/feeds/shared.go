package feeds

import "strings"

// sharedHosts are platforms where anyone can host content. A malicious URL
// on them says nothing about other URLs on the same host, so they are kept
// out of the host-level sets (exact URL matches still apply).
var sharedHosts = []string{
	"github.com", "raw.githubusercontent.com", "gist.githubusercontent.com", "objects.githubusercontent.com",
	"gitlab.com", "bitbucket.org",
	"drive.google.com", "docs.google.com", "sites.google.com", "storage.googleapis.com",
	"firebasestorage.googleapis.com", "googleusercontent.com", "blogspot.com", "web.app", "firebaseapp.com",
	"dropbox.com", "dl.dropboxusercontent.com", "onedrive.live.com", "1drv.ms", "sharepoint.com",
	"mediafire.com", "mega.nz", "discord.com", "cdn.discordapp.com", "telegra.ph", "pastebin.com",
	"bit.ly", "t.co", "tinyurl.com", "s3.amazonaws.com", "amazonaws.com", "cloudfront.net",
	"azurewebsites.net", "blob.core.windows.net", "herokuapp.com", "netlify.app", "vercel.app",
	"pages.dev", "workers.dev", "glitch.me", "replit.dev", "weebly.com", "wixsite.com", "000webhostapp.com",
	"ngrok-free.app", "ngrok.io", "trycloudflare.com",
}

// IsSharedHost reports whether host is, or is under, a shared platform.
func IsSharedHost(host string) bool {
	host = strings.ToLower(host)
	for _, s := range sharedHosts {
		if host == s || strings.HasSuffix(host, "."+s) {
			return true
		}
	}
	return false
}
