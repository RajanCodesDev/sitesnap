package sitemap

import "encoding/xml"

// URLSet represents a standard sitemap.xml file.
//
// Example:
//
//	<urlset>
//	    <url>
//	        <loc>https://example.com/</loc>
//	    </url>
//	</urlset>
type URLSet struct {
	XMLName xml.Name `xml:"urlset"`
	URLs    []URL    `xml:"url"`
}

// URL represents a single URL entry.
type URL struct {
	Loc string `xml:"loc"`
}

// SitemapIndex represents a sitemap index.
//
// Example:
//
//	<sitemapindex>
//	    <sitemap>
//	        <loc>https://example.com/products.xml</loc>
//	    </sitemap>
//	</sitemapindex>
type SitemapIndex struct {
	XMLName  xml.Name  `xml:"sitemapindex"`
	Sitemaps []Sitemap `xml:"sitemap"`
}

// Sitemap represents a child sitemap.
type Sitemap struct {
	Loc string `xml:"loc"`
}
