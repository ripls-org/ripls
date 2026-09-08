package webfetch

import (
	"net/url"
	"testing"
)

func TestUpgradeImageURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		// Amazon: strip the ._SY###_ / ._SL###_ / ._AC_UY###_ token.
		{
			name: "amazon SY token",
			in:   "https://m.media-amazon.com/images/I/61abc123._SY300_.jpg",
			want: "https://m.media-amazon.com/images/I/61abc123.jpg",
		},
		{
			name: "amazon SL token",
			in:   "https://m.media-amazon.com/images/I/61abc123._SL1500_.jpg",
			want: "https://m.media-amazon.com/images/I/61abc123.jpg",
		},
		{
			name: "amazon AC_UY combo token",
			in:   "https://m.media-amazon.com/images/I/61abc123._AC_UY300_.jpg",
			want: "https://m.media-amazon.com/images/I/61abc123.jpg",
		},
		{
			name: "amazon multi-modifier token",
			in:   "https://m.media-amazon.com/images/I/61abc123._UX466_FMjpg_QL85_.jpg",
			want: "https://m.media-amazon.com/images/I/61abc123.jpg",
		},
		{
			name: "amazon ssl-images host also matches",
			in:   "https://images-na.ssl-images-amazon.com/images/I/abc._SX522_.jpg",
			want: "https://images-na.ssl-images-amazon.com/images/I/abc.jpg",
		},
		{
			name: "amazon already-master no-op",
			in:   "https://m.media-amazon.com/images/I/61abc123.jpg",
			want: "https://m.media-amazon.com/images/I/61abc123.jpg",
		},
		{
			name: "amazon png token strip",
			in:   "https://m.media-amazon.com/images/I/61abc._SX300_.png",
			want: "https://m.media-amazon.com/images/I/61abc.png",
		},

		// Shopify: drop _WxH(_crop_*)? suffix and width=/height= query.
		{
			name: "shopify _W only suffix",
			in:   "https://cdn.shopify.com/s/files/1/0123/4567/products/widget_300x.jpg",
			want: "https://cdn.shopify.com/s/files/1/0123/4567/products/widget.jpg",
		},
		{
			name: "shopify _WxH suffix",
			in:   "https://cdn.shopify.com/s/files/1/0123/4567/products/widget_600x600.jpg",
			want: "https://cdn.shopify.com/s/files/1/0123/4567/products/widget.jpg",
		},
		{
			name: "shopify with crop suffix",
			in:   "https://cdn.shopify.com/s/files/1/0123/4567/products/widget_600x600_crop_center.jpg",
			want: "https://cdn.shopify.com/s/files/1/0123/4567/products/widget.jpg",
		},
		{
			name: "shopify width query gets dropped (cache key v= preserved)",
			in:   "https://cdn.shopify.com/s/files/1/0123/4567/products/widget.jpg?v=12345&width=300",
			want: "https://cdn.shopify.com/s/files/1/0123/4567/products/widget.jpg?v=12345",
		},
		{
			name: "shopify subdomain also matches",
			in:   "https://example-store.myshopify.com/cdn/shop/products/widget_300x.jpg",
			want: "https://example-store.myshopify.com/cdn/shop/products/widget.jpg",
		},
		{
			name: "shopify plus on brand domain (path-based match)",
			in:   "https://www.allbirds.com/cdn/shop/files/widget.png?v=123&width=100",
			want: "https://www.allbirds.com/cdn/shop/files/widget.png?v=123",
		},
		{
			name: "shopify plus path-based match with size suffix",
			in:   "https://www.brand.com/cdn/shop/files/widget_600x600.jpg?v=123",
			want: "https://www.brand.com/cdn/shop/files/widget.jpg?v=123",
		},
		{
			name: "shopify already-master no-op",
			in:   "https://cdn.shopify.com/s/files/1/0/products/widget.jpg",
			want: "https://cdn.shopify.com/s/files/1/0/products/widget.jpg",
		},

		// Etsy: il_<W>x<H>. → il_fullxfull.
		{
			name: "etsy il_300xN",
			in:   "https://i.etsystatic.com/12345678/r/il/il_300xN.4567890123_abc.jpg",
			want: "https://i.etsystatic.com/12345678/r/il/il_fullxfull.4567890123_abc.jpg",
		},
		{
			name: "etsy il_300x240",
			in:   "https://i.etsystatic.com/12345678/r/il/il_300x240.4567890123_abc.jpg",
			want: "https://i.etsystatic.com/12345678/r/il/il_fullxfull.4567890123_abc.jpg",
		},
		{
			name: "etsy already fullxfull no-op",
			in:   "https://i.etsystatic.com/12345678/r/il/il_fullxfull.4567890123_abc.jpg",
			want: "https://i.etsystatic.com/12345678/r/il/il_fullxfull.4567890123_abc.jpg",
		},

		// Target / scene7: drop wid/hei/qlt query.
		{
			name: "target scene7 wid+hei",
			in:   "https://target.scene7.com/is/image/Target/GUEST_abc123?wid=300&hei=300",
			want: "https://target.scene7.com/is/image/Target/GUEST_abc123",
		},
		{
			name: "target scene7 qlt also dropped",
			in:   "https://target.scene7.com/is/image/Target/GUEST_abc123?wid=300&hei=300&qlt=75&fmt=webp",
			want: "https://target.scene7.com/is/image/Target/GUEST_abc123?fmt=webp",
		},
		{
			name: "target.com host also matches",
			in:   "https://www.target.com/is/image/abc?wid=300",
			want: "https://www.target.com/is/image/abc",
		},
		{
			name: "scene7 no size query no-op",
			in:   "https://target.scene7.com/is/image/Target/GUEST_abc",
			want: "https://target.scene7.com/is/image/Target/GUEST_abc",
		},

		// Substack CDN: unwrap the percent-encoded master URL when the master
		// is a mobile-friendly aspect ratio.
		{
			name: "substack hero image — unwrap to original 3418x2279 master (1.5:1, fine)",
			in:   "https://substackcdn.com/image/fetch/$s_!aVxJ!,w_1200,h_675,c_fill,f_jpg,q_auto:good,fl_progressive:steep,g_auto/https%3A%2F%2Fsubstack-post-media.s3.amazonaws.com%2Fpublic%2Fimages%2Fc4c64f00-a605-4c8c-8d6c-80bd82585783_3418x2279.jpeg",
			want: "https://substack-post-media.s3.amazonaws.com/public/images/c4c64f00-a605-4c8c-8d6c-80bd82585783_3418x2279.jpeg",
		},
		{
			name: "substack banner master 5830x1411 (4.13:1) keeps CDN 1200x675 — wide banner crops badly on mobile",
			in:   "https://substackcdn.com/image/fetch/$s_!xx!,w_1200,h_675,c_fill/https%3A%2F%2Fsubstack-post-media.s3.amazonaws.com%2Fpublic%2Fimages%2Fbanner_5830x1411.png",
			want: "https://substackcdn.com/image/fetch/$s_!xx!,w_1200,h_675,c_fill/https%3A%2F%2Fsubstack-post-media.s3.amazonaws.com%2Fpublic%2Fimages%2Fbanner_5830x1411.png",
		},
		{
			name: "substack skyscraper master 800x4000 (0.2:1) keeps CDN — tall image crops badly too",
			in:   "https://substackcdn.com/image/fetch/$s_!yy!,w_400,h_675/https%3A%2F%2Fsubstack-post-media.s3.amazonaws.com%2Fpublic%2Fimages%2Ftall_800x4000.png",
			want: "https://substackcdn.com/image/fetch/$s_!yy!,w_400,h_675/https%3A%2F%2Fsubstack-post-media.s3.amazonaws.com%2Fpublic%2Fimages%2Ftall_800x4000.png",
		},
		{
			name: "substack square master 2000x2000 — unwrap (perfectly mobile-friendly)",
			in:   "https://substackcdn.com/image/fetch/$s_!zz!,w_1200,h_675/https%3A%2F%2Fsubstack-post-media.s3.amazonaws.com%2Fpublic%2Fimages%2Fphoto_2000x2000.jpeg",
			want: "https://substack-post-media.s3.amazonaws.com/public/images/photo_2000x2000.jpeg",
		},
		{
			name: "substack cdn without encoded suffix — no-op",
			in:   "https://substackcdn.com/image/fetch/some/non-substack-path.jpg",
			want: "https://substackcdn.com/image/fetch/some/non-substack-path.jpg",
		},
		{
			name: "substack cdn with http-encoded suffix and no dimension hint — unwrap (no reason to reject)",
			in:   "https://substackcdn.com/image/fetch/w_800/http%3A%2F%2Fexample.com%2Fimage.png",
			want: "http://example.com/image.png",
		},

		// Unknown hosts: identity.
		{
			name: "unknown host left alone",
			in:   "https://example.com/path/to/image_300x300.jpg",
			want: "https://example.com/path/to/image_300x300.jpg",
		},
		{
			name: "amazon-lookalike does not match",
			in:   "https://fake-amazon.com/I/abc._SY300_.jpg",
			want: "https://fake-amazon.com/I/abc._SY300_.jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, err := url.Parse(tt.in)
			if err != nil {
				t.Fatalf("parse input: %v", err)
			}
			got := upgradeImageURL(in).String()
			if got != tt.want {
				t.Errorf("upgradeImageURL(%q)\n  got:  %q\n  want: %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestUpgradeImageURL_NilOrEmpty(t *testing.T) {
	if got := upgradeImageURL(nil); got != nil {
		t.Errorf("nil input should return nil, got %v", got)
	}
	u := &url.URL{Scheme: "https"}
	if got := upgradeImageURL(u); got != u {
		t.Errorf("empty-host input should be returned unchanged")
	}
}

func TestHostMatches(t *testing.T) {
	tests := []struct {
		host, suffix string
		want         bool
	}{
		{"amazon.com", "amazon.com", true},
		{"www.amazon.com", "amazon.com", true},
		{"m.media-amazon.com", "media-amazon.com", true},
		{"media-amazon.com", "media-amazon.com", true},
		{"fakeamazon.com", "amazon.com", false}, // no leading dot, must not match
		{"example.com", "amazon.com", false},
	}
	for _, tt := range tests {
		if got := hostMatches(tt.host, tt.suffix); got != tt.want {
			t.Errorf("hostMatches(%q, %q) = %v, want %v", tt.host, tt.suffix, got, tt.want)
		}
	}
}
