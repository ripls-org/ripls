package webfetch

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Mobile-preview aspect-ratio guards. Anything wider than 2.5:1 or taller
// than 1:2.5 reads as a desktop banner / skyscraper when cropped to a
// portrait card and produces visibly blurry text or oddly cropped subjects.
// Use these to prefer a smaller-but-better-composed CDN variant over a
// huge but mis-shaped master.
const (
	maxMobileFriendlyAspect = 2.5
	minMobileFriendlyAspect = 1.0 / maxMobileFriendlyAspect
)

// upgradeImageURL rewrites known e-commerce image URLs so the path/query
// references the high-resolution master variant instead of the small
// link-preview thumbnail typically returned via og:image. Returns the input
// unchanged when the host/path isn't recognized or no rewrite applies.
//
// The rewrites are deterministic and require no extra HTTP — each entry knows
// the resolution-token shape its host uses. If a rewrite produces a broken
// URL (e.g., because the site changed its scheme), downloadAndStoreWebpageImage
// already handles fetch failure and falls through to the original og:image.
//
// Both host-suffix and path-prefix matches are honored; Shopify Plus
// customers serve CDN URLs from their own domain (e.g.
// www.allbirds.com/cdn/shop/...) and would otherwise miss the host check.
func upgradeImageURL(u *url.URL) *url.URL {
	if u == nil || u.Host == "" {
		return u
	}
	host := strings.ToLower(u.Host)
	for _, r := range domainRewrites {
		if hostMatches(host, r.hostSuffix) {
			return r.rewrite(u)
		}
	}
	path := u.Path
	for _, r := range pathRewrites {
		if strings.HasPrefix(path, r.pathPrefix) {
			return r.rewrite(u)
		}
	}
	return u
}

// hostMatches reports whether host equals suffix or ends with "." + suffix
// (so "media-amazon.com" matches "i.media-amazon.com" but not
// "fakemedia-amazon.com").
func hostMatches(host, suffix string) bool {
	return host == suffix || strings.HasSuffix(host, "."+suffix)
}

type domainRewrite struct {
	hostSuffix string
	rewrite    func(*url.URL) *url.URL
}

// Ordering: most-specific first when two suffixes could both match (none today).
var domainRewrites = []domainRewrite{
	{"amazon.com", stripAmazonSizeToken},
	{"media-amazon.com", stripAmazonSizeToken},
	{"ssl-images-amazon.com", stripAmazonSizeToken},
	{"shopify.com", stripShopifySize},
	{"myshopify.com", stripShopifySize}, // per-store hostnames
	{"etsystatic.com", etsyToFullxFull},
	{"target.com", stripScene7SizeQuery},
	{"scene7.com", stripScene7SizeQuery},
	{"substackcdn.com", unwrapSubstackCDN},
}

type pathRewrite struct {
	pathPrefix string
	rewrite    func(*url.URL) *url.URL
}

// pathRewrites cover platforms whose CDN URL shape is identifiable from the
// path rather than the host. Shopify Plus customers serve images from their
// own domains via /cdn/shop/... — without this rule the host-suffix match
// would miss them entirely.
var pathRewrites = []pathRewrite{
	{"/cdn/shop/", stripShopifySize},
	{"/cdn/shopify/", stripShopifySize},
}

// Amazon image paths embed a size/treatment modifier between two underscores
// before the extension: "61abc._SY300_.jpg", "61abc._AC_UY300_.jpg",
// "61abc._UX466_FMjpg_QL85_.jpg". The modifier always starts with an
// uppercase letter but may contain lowercase chars (e.g. "FMjpg"). Stripping
// it returns the natural-resolution master.
var amazonSizeTokenRE = regexp.MustCompile(`\._[A-Z][A-Za-z0-9_]*_\.(jpg|jpeg|png|webp|gif)$`)

func stripAmazonSizeToken(u *url.URL) *url.URL {
	out := *u
	out.Path = amazonSizeTokenRE.ReplaceAllString(u.Path, ".$1")
	return &out
}

// Shopify CDN URLs encode the variant size as a "_WxH" or "_W x" suffix
// before the extension, optionally followed by a "_crop_<dir>" treatment:
// "widget_300x.jpg", "widget_300x300.jpg", "widget_600x600_crop_center.jpg".
// They can also carry a "?width=..." query.
var shopifySizeSuffixRE = regexp.MustCompile(`_\d+x(\d+)?(_[a-z_]+)?\.(jpg|jpeg|png|webp|gif)$`)

func stripShopifySize(u *url.URL) *url.URL {
	out := *u
	out.Path = shopifySizeSuffixRE.ReplaceAllString(u.Path, ".$3")
	q := out.Query()
	q.Del("width")
	q.Del("height")
	out.RawQuery = q.Encode()
	return &out
}

// Etsy CDN URLs encode the variant as "il_<W>x<H>." or "il_<W>xN." early in
// the filename: "il_300xN.4567890123_abc.jpg". The "fullxfull" variant is
// the original-resolution master.
var etsySizeTokenRE = regexp.MustCompile(`il_\d+x[\dN]+\.`)

func etsyToFullxFull(u *url.URL) *url.URL {
	out := *u
	out.Path = etsySizeTokenRE.ReplaceAllString(u.Path, "il_fullxfull.")
	return &out
}

// Target's image CDN (scene7) uses query params for sizing: "?wid=300&hei=300".
// Dropping both yields the natural-resolution master that the product page
// itself displays inline.
func stripScene7SizeQuery(u *url.URL) *url.URL {
	out := *u
	q := out.Query()
	q.Del("wid")
	q.Del("hei")
	q.Del("qlt")
	out.RawQuery = q.Encode()
	return &out
}

// Substack wraps every image through their CDN with a path like
//
//	/image/fetch/$s_<hash>,w_1200,h_675,c_fill,.../<percent-encoded-master-url>
//
// The trailing segment — once Go's url package has decoded the percent-
// escaping in u.Path — is a literal "https://substack-post-media.s3.
// amazonaws.com/..._<W>x<H>.<ext>" URL. Unwrapping it bypasses the CDN's
// transform parameters (which cap the og:image at 1200×675 by default)
// and points at the original-resolution master.
//
// Aspect-ratio guard: Substack's master filenames encode the original
// dimensions as "_<W>x<H>.<ext>". When the master is an extreme aspect
// (a desktop banner or skyscraper, e.g. 5830×1411 = 4.13:1) the CDN-sized
// 1200×675 version is composed for social sharing and crops better on a
// portrait card. In that case we keep the CDN URL.
var (
	substackInlineURLRE  = regexp.MustCompile(`https?://.+$`)
	substackMasterDimsRE = regexp.MustCompile(`_(\d+)x(\d+)\.[A-Za-z]+$`)
)

func unwrapSubstackCDN(u *url.URL) *url.URL {
	inline := substackInlineURLRE.FindString(u.Path)
	if inline == "" {
		return u
	}
	parsed, err := url.Parse(inline)
	if err != nil || (parsed.Scheme != schemeHTTP && parsed.Scheme != schemeHTTPS) {
		return u
	}
	if !mobileFriendlyAspect(parsed.Path) {
		return u
	}
	return parsed
}

// mobileFriendlyAspect reports whether an embedded "_<W>x<H>.<ext>" filename
// suggests a sane aspect ratio for a mobile preview card. Returns true when
// the filename carries no dimensions (we have no reason to reject) or when
// the dimensions fall within the [minMobileFriendlyAspect,
// maxMobileFriendlyAspect] band.
func mobileFriendlyAspect(path string) bool {
	dims := substackMasterDimsRE.FindStringSubmatch(path)
	if dims == nil {
		return true
	}
	w, _ := strconv.Atoi(dims[1])
	h, _ := strconv.Atoi(dims[2])
	if w <= 0 || h <= 0 {
		return true
	}
	ratio := float64(w) / float64(h)
	return ratio >= minMobileFriendlyAspect && ratio <= maxMobileFriendlyAspect
}
