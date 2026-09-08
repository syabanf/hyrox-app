// Command gen writes the demo images into internal/media/assets.
//
//	go run ./internal/media/gen
//
// It is deterministic: the same name always produces the same picture, so
// regenerating never churns the diff, and a new image is a new file rather
// than a change to an old one.
package main

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
)

// The brand palette. Every image is built from these, so the demo does not
// look like it was assembled from whatever was lying around.
const (
	forest  = "#00281A"
	lime    = "#DAFF59"
	lettuce = "#B9E63F"
	beige   = "#F3ECE2"
	white   = "#FFFFFF"
)

// accents give each subject its own colour without leaving the palette.
var accents = [][2]string{
	{"#00281A", "#0C4A31"},
	{"#0C4A31", "#1B6B47"},
	{"#1B6B47", "#37855C"},
	{"#123B57", "#1E5C7E"},
	{"#4A2C13", "#7A4A20"},
	{"#3A1D3F", "#5E2F66"},
}

// pick chooses deterministically from a list, by name.
func pick[T any](name string, options []T) T {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return options[int(h.Sum32())%len(options)]
}

func initials(label string) string {
	parts := strings.Fields(label)
	out := ""
	for _, part := range parts {
		if len(out) >= 2 {
			break
		}
		out += strings.ToUpper(part[:1])
	}
	if out == "" {
		return "N"
	}
	return out
}

// avatar is a round portrait stand-in: a brand gradient with initials on it.
func avatar(name, label string) string {
	pair := pick(name, accents)
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 160 160" width="160" height="160" role="img" aria-label="%s">
  <defs>
    <linearGradient id="g" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0" stop-color="%s"/><stop offset="1" stop-color="%s"/>
    </linearGradient>
  </defs>
  <rect width="160" height="160" rx="80" fill="url(#g)"/>
  <text x="80" y="80" fill="%s" font-family="Outfit, Manrope, system-ui, sans-serif"
        font-size="58" font-weight="800" text-anchor="middle" dominant-baseline="central">%s</text>
</svg>
`, escape(label), pair[0], pair[1], lime, initials(label))
}

// photo is a wide stand-in for a photograph: a brand gradient under the
// pebble texture, with a dark foot so white type sits legibly on it.
//
// It carries no text of its own. Every screen that uses one of these draws the
// name over the top — a race card, an announcement — and a picture with the
// name baked in shows it twice, offset, which reads as a rendering bug.
func photo(name, label string) string {
	pair := pick(name, accents)
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 960 540" width="960" height="540" role="img" aria-label="%s">
  <defs>
    <linearGradient id="g" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0" stop-color="%s"/><stop offset="1" stop-color="%s"/>
    </linearGradient>
    <linearGradient id="foot" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#000000" stop-opacity="0"/>
      <stop offset="1" stop-color="#000000" stop-opacity="0.45"/>
    </linearGradient>
    <pattern id="p" width="120" height="80" patternUnits="userSpaceOnUse" patternTransform="rotate(-12)">
      <ellipse cx="60" cy="40" rx="46" ry="17" fill="%s" opacity="0.10"/>
      <ellipse cx="0" cy="10" rx="46" ry="17" fill="%s" opacity="0.06"/>
    </pattern>
  </defs>
  <rect width="960" height="540" fill="url(#g)"/>
  <rect width="960" height="540" fill="url(#p)"/>
  <rect y="240" width="960" height="300" fill="url(#foot)"/>
</svg>
`, escape(label), pair[0], pair[1], lime, white)
}

// product is a square stand-in for a shelf photograph.
func product(name, label string) string {
	pair := pick(name, accents)
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 400 400" width="400" height="400" role="img" aria-label="%s">
  <defs>
    <linearGradient id="g" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="%s"/><stop offset="1" stop-color="%s"/>
    </linearGradient>
  </defs>
  <rect width="400" height="400" rx="28" fill="%s"/>
  <rect x="112" y="70" width="176" height="230" rx="24" fill="url(#g)"/>
  <rect x="112" y="70" width="176" height="56" rx="24" fill="%s" opacity="0.85"/>
  <text x="200" y="348" fill="%s" font-family="Outfit, Manrope, system-ui, sans-serif"
        font-size="26" font-weight="800" text-anchor="middle">%s</text>
</svg>
`, escape(label), pair[0], pair[1], beige, lettuce, forest, escape(truncate(label, 18)))
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}

func escape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

type image struct {
	file string
	body string
}

func main() {
	dir := filepath.Join("internal", "media", "assets")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	images := []image{
		// Members and staff.
		{"avatar-fahmi.svg", avatar("fahmi", "Fahmi Syaban")},
		{"avatar-natalie.svg", avatar("natalie", "Natalie Brown")},
		{"avatar-lucas.svg", avatar("lucas", "Lucas Conn")},
		{"avatar-jaime.svg", avatar("jaime", "Jaime Waelchi")},
		{"avatar-doris.svg", avatar("doris", "Doris Conroy")},
		{"avatar-luke.svg", avatar("luke", "Luke Nader")},
		{"avatar-sari.svg", avatar("sari", "Sari Wulandari")},
		{"avatar-dimas.svg", avatar("dimas", "Dimas Anggara")},
		{"avatar-rina.svg", avatar("rina", "Rina Kartika")},
		{"avatar-kevin.svg", avatar("kevin", "Kevin Hartono")},
		{"avatar-maya.svg", avatar("maya", "Maya Kusuma")},
		{"avatar-rizky.svg", avatar("rizky", "Rizky Ramadhan")},
		{"avatar-tara.svg", avatar("tara", "Tara Widjaja")},
		{"avatar-alya.svg", avatar("alya", "Alya Santoso")},
		{"avatar-raka.svg", avatar("raka", "Raka Wibowo")},
		{"avatar-bima.svg", avatar("bima", "Bima Prasetyo")},
		{"avatar-nadia.svg", avatar("nadia", "Nadia Putri")},
		{"avatar-sinta.svg", avatar("sinta", "Sinta Halim")},

		// Races.
		{"race-jakarta.svg", photo("jakarta", "Jakarta")},
		{"race-singapore.svg", photo("singapore", "Singapore")},
		{"race-bangkok.svg", photo("bangkok", "Bangkok")},
		{"race-sydney.svg", photo("sydney", "Sydney")},
		{"race-london.svg", photo("london", "London")},

		// Campaigns and rewards.
		{"campaign-welcome.svg", photo("welcome", "Welcome back")},
		{"campaign-race.svg", photo("racecamp", "Race season")},
		{"reward-shaker.svg", product("rwdshaker", "Free Shaker")},
		{"reward-bar.svg", product("rwdbar", "Free Bar")},
		{"reward-class.svg", product("rwdclass", "One Free Class")},
		{"reward-tee.svg", product("rwdtee", "Free Tee")},
		{"reward-month.svg", product("rwdmonth", "Free Month")},

		// The shelf. One picture per stock item; the till's products point at
		// the same files, because a carton of a drink is a picture of a drink.
		{"item-tee.svg", product("tee", "Training Tee")},
		{"item-tank.svg", product("tank", "Tank Top")},
		{"item-bottle.svg", product("bottle750", "Steel Bottle")},
		{"item-grips.svg", product("grips", "Training Grips")},
		{"item-shaker.svg", product("shaker", "Protein Shaker")},
		{"item-bar.svg", product("proteinbar", "Protein Bar")},
		{"item-iso.svg", product("isotonic", "Isotonic Drink")},
		{"item-whey.svg", product("whey", "Whey Protein")},
		{"item-towel.svg", product("gymtowel", "Gym Towel")},
		{"item-wipes.svg", product("wipes", "Equipment Wipes")},
	}

	for _, img := range images {
		path := filepath.Join(dir, img.file)
		if err := os.WriteFile(path, []byte(img.body), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	fmt.Printf("wrote %d images to %s\n", len(images), dir)
}
