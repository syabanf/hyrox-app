package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"
)

// The athlete side of the demo: recorded activities with real GPS tracks, the
// social graph around them, segments to race, clubs to join, and a race
// calendar to train for.
//
// The tracks are generated rather than recorded, but they are generated the
// way a track actually looks — a route with corners, sampled every few
// seconds, at a pace that varies — so the stats, splits and segment matching
// exercise the same code a phone's track would.

// Jakarta, around the two branches.
const (
	senopatiLat = -6.2380
	senopatiLng = 106.8100
	pikLat      = -6.1090
	pikLng      = 106.7400
)

// metresPerDegree is close enough at this latitude for a demo track.
const metresPerDegree = 111_320.0

type trackPoint struct {
	T   int64   `json:"t"`
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
	Ele float64 `json:"ele"`
}

// loopTrack draws an out-and-back along a bearing, sampled every five seconds
// at the given pace. Out and back rather than a straight line so a heatmap
// shows something a runner would recognise.
func loopTrack(startLat, startLng float64, distanceM float64, paceSecPerKm int, bearing float64) []trackPoint {
	const sampleSec = 5
	speed := 1000.0 / float64(paceSecPerKm)
	step := speed * sampleSec
	samples := int(distanceM / step)
	if samples < 2 {
		samples = 2
	}
	half := samples / 2

	points := make([]trackPoint, 0, samples+1)
	lat, lng := startLat, startLng
	for i := 0; i <= samples; i++ {
		// Turn around at halfway, and drift the bearing so the line is not
		// dead straight.
		heading := bearing + float64(i)*0.6
		if i > half {
			heading = bearing + 180 + float64(samples-i)*0.6
		}
		rad := heading * math.Pi / 180
		lat += step * math.Cos(rad) / metresPerDegree
		lng += step * math.Sin(rad) / (metresPerDegree * math.Cos(startLat*math.Pi/180))

		points = append(points, trackPoint{
			T:   int64(i) * sampleSec * 1000,
			Lat: lat,
			Lng: lng,
			// A gentle rise and fall, so elevation gain is not zero.
			Ele: 12 + 4*math.Sin(float64(i)/8),
		})
	}
	return points
}

func (s *Seeder) seedTraining(ctx context.Context) (int, error) {
	now := s.clock.Now()

	// ── Segments ─────────────────────────────────────────────────────────────
	// Each segment is the first kilometre of one of the seeded routes, so the
	// activities that run it actually match it.
	senopatiRun := loopTrack(senopatiLat, senopatiLng, 8000, 330, 45)
	pikRun := loopTrack(pikLat, pikLng, 10000, 300, 200)

	segments := []struct {
		id, name, kind, location string
		distanceM                float64
		path                     []trackPoint
	}{
		{"seg_senopati_mile", "Senopati Mile", "RUN", "Senopati", 1000,
			[]trackPoint{senopatiRun[0], senopatiRun[40]}},
		{"seg_pik_seawall", "PIK Seawall Sprint", "RUN", "Pantai Indah Kapuk", 1000,
			[]trackPoint{pikRun[0], pikRun[30]}},
	}
	for _, seg := range segments {
		path, err := json.Marshal(seg.path)
		if err != nil {
			return 0, fmt.Errorf("seed: encoding segment %s: %w", seg.id, err)
		}
		if _, err := s.db.Exec(ctx, `
			INSERT INTO training.segments (id, name, type, distance_m, location, path)
			VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (id) DO NOTHING`,
			seg.id, seg.name, seg.kind, seg.distanceM, seg.location, path); err != nil {
			return 0, fmt.Errorf("seed: segment %s: %w", seg.id, err)
		}
	}

	// ── Gear ─────────────────────────────────────────────────────────────────
	gear := []struct{ id, member, name, kind string }{
		{"gea_demo_shoes", "mem_demo", "Nike Pegasus 41", "SHOES"},
		{"gea_demo_race", "mem_demo", "Adidas Adizero (race day)", "SHOES"},
		{"gea_lucas_bike", "mem_lucas", "Polygon Strattos", "BIKE"},
	}
	for _, g := range gear {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO training.gear (id, member_id, name, kind)
			VALUES ($1,$2,$3,$4) ON CONFLICT (id) DO NOTHING`,
			g.id, g.member, g.name, g.kind); err != nil {
			return 0, fmt.Errorf("seed: gear %s: %w", g.id, err)
		}
	}

	// ── Activities ───────────────────────────────────────────────────────────
	activities := []struct {
		id, member, kind, title string
		daysAgo                 int
		hour                    int
		distanceM               float64
		paceSecPerKm            int
		bearing                 float64
		fromSenopati            bool
		visibility              string
		gearID                  *string
	}{
		{"act_demo_long", "mem_demo", "RUN", "Sunday long run", 3, 6, 12_000, 340, 45, true, "EVERYONE", strPtr("gea_demo_shoes")},
		{"act_demo_tempo", "mem_demo", "RUN", "Tempo along Senopati", 5, 18, 8_000, 300, 45, true, "EVERYONE", strPtr("gea_demo_shoes")},
		{"act_demo_easy", "mem_demo", "RUN", "Easy shakeout", 9, 6, 5_000, 380, 45, true, "FOLLOWERS", strPtr("gea_demo_shoes")},
		{"act_demo_walk", "mem_demo", "WALK", "Walk to the studio", 12, 7, 2_000, 700, 45, true, "PRIVATE", nil},
		{"act_demo_older", "mem_demo", "RUN", "Half marathon effort", 16, 6, 21_100, 350, 45, true, "EVERYONE", strPtr("gea_demo_race")},
		{"act_natalie_seawall", "mem_natalie", "RUN", "Seawall repeats", 3, 6, 10_000, 300, 200, false, "EVERYONE", nil},
		{"act_natalie_easy", "mem_natalie", "RUN", "Recovery jog", 6, 17, 6_000, 400, 200, false, "EVERYONE", nil},
		{"act_lucas_ride", "mem_lucas", "RIDE", "Morning loop", 2, 6, 32_000, 120, 45, true, "EVERYONE", strPtr("gea_lucas_bike")},
		{"act_lucas_run", "mem_lucas", "RUN", "Brick run off the bike", 2, 8, 5_000, 330, 45, true, "EVERYONE", nil},
		{"act_jaime_run", "mem_jaime", "RUN", "Senopati mile repeats", 4, 6, 7_000, 320, 45, true, "EVERYONE", nil},
		{"act_sari_first", "mem_sari", "RUN", "First run back", 1, 18, 4_000, 420, 45, true, "EVERYONE", nil},
		{"act_doris_walk", "mem_doris", "WALK", "Evening walk", 2, 19, 3_500, 720, 200, false, "EVERYONE", nil},
	}

	inserted := 0
	for _, a := range activities {
		lat, lng := senopatiLat, senopatiLng
		if !a.fromSenopati {
			lat, lng = pikLat, pikLng
		}
		points := loopTrack(lat, lng, a.distanceM, a.paceSecPerKm, a.bearing)
		encoded, err := json.Marshal(points)
		if err != nil {
			return 0, fmt.Errorf("seed: encoding track %s: %w", a.id, err)
		}

		startedAt := time.Date(now.Year(), now.Month(), now.Day(), a.hour, 12, 0, 0, now.Location()).
			AddDate(0, 0, -a.daysAgo)
		elapsed := len(points) * 5
		// A minute of the run was spent at traffic lights, which is the point
		// of keeping moving time separate from elapsed.
		moving := elapsed - 60
		pace := int(float64(moving) / (a.distanceM / 1000))

		tag, err := s.db.Exec(ctx, `
			INSERT INTO training.activities
				(id, member_id, type, title, started_at, elapsed_sec, moving_sec, distance_m,
				 avg_pace_sec_per_km, elevation_gain_m, points, photos, visibility, gear_id, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'[]'::jsonb,$12,$13,$5)
			ON CONFLICT (id) DO NOTHING`,
			a.id, a.member, a.kind, a.title, startedAt, elapsed, moving, a.distanceM,
			pace, 24.0, encoded, a.visibility, a.gearID)
		if err != nil {
			return 0, fmt.Errorf("seed: activity %s: %w", a.id, err)
		}
		inserted += int(tag.RowsAffected())
	}

	// Shoe mileage follows the activities logged against them.
	if _, err := s.db.Exec(ctx, `
		UPDATE training.gear g SET distance_m = coalesce((
			SELECT sum(a.distance_m) FROM training.activities a WHERE a.gear_id = g.id
		), 0)`); err != nil {
		return 0, fmt.Errorf("seed: gear mileage: %w", err)
	}

	// ── Segment efforts ──────────────────────────────────────────────────────
	// Recorded rather than matched, so the leaderboard has a field on it
	// without the seeder having to run the matcher.
	efforts := []struct {
		id, segment, activity, member string
		elapsedSec                    int
	}{
		{"eff_demo_1", "seg_senopati_mile", "act_demo_tempo", "mem_demo", 298},
		{"eff_demo_2", "seg_senopati_mile", "act_demo_long", "mem_demo", 336},
		{"eff_jaime_1", "seg_senopati_mile", "act_jaime_run", "mem_jaime", 312},
		{"eff_lucas_1", "seg_senopati_mile", "act_lucas_run", "mem_lucas", 327},
		{"eff_natalie_1", "seg_pik_seawall", "act_natalie_seawall", "mem_natalie", 289},
		{"eff_natalie_2", "seg_pik_seawall", "act_natalie_easy", "mem_natalie", 401},
	}
	for _, e := range efforts {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO training.segment_efforts (id, segment_id, activity_id, member_id, elapsed_sec)
			VALUES ($1,$2,$3,$4,$5) ON CONFLICT (activity_id, segment_id) DO NOTHING`,
			e.id, e.segment, e.activity, e.member, e.elapsedSec); err != nil {
			return 0, fmt.Errorf("seed: effort %s: %w", e.id, err)
		}
	}

	// ── The social graph ─────────────────────────────────────────────────────
	follows := [][2]string{
		{"mem_demo", "mem_natalie"}, {"mem_demo", "mem_lucas"},
		{"mem_natalie", "mem_demo"}, {"mem_jaime", "mem_demo"},
		{"mem_lucas", "mem_demo"}, {"mem_sari", "mem_demo"},
		{"mem_lucas", "mem_natalie"},
	}
	for _, f := range follows {
		if _, err := s.db.Exec(ctx,
			`INSERT INTO training.follows (follower_id, followee_id) VALUES ($1,$2)
			 ON CONFLICT DO NOTHING`, f[0], f[1]); err != nil {
			return 0, fmt.Errorf("seed: follow %s→%s: %w", f[0], f[1], err)
		}
	}

	kudos := [][2]string{
		{"act_demo_long", "mem_natalie"}, {"act_demo_long", "mem_lucas"},
		{"act_demo_long", "mem_jaime"}, {"act_demo_tempo", "mem_natalie"},
		{"act_natalie_seawall", "mem_demo"}, {"act_lucas_ride", "mem_demo"},
	}
	for _, k := range kudos {
		if _, err := s.db.Exec(ctx,
			`INSERT INTO training.kudos (activity_id, member_id) VALUES ($1,$2)
			 ON CONFLICT DO NOTHING`, k[0], k[1]); err != nil {
			return 0, fmt.Errorf("seed: kudos on %s: %w", k[0], err)
		}
	}

	comments := []struct{ id, activity, member, text string }{
		{"cmt_demo_1", "act_demo_long", "mem_natalie", "That last 5k was quick. See you Sunday?"},
		{"cmt_demo_2", "act_demo_long", "mem_lucas", "Strong."},
		{"cmt_demo_3", "act_natalie_seawall", "mem_demo", "Those repeats look brutal."},
	}
	for _, c := range comments {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO training.activity_comments (id, activity_id, member_id, text)
			VALUES ($1,$2,$3,$4) ON CONFLICT (id) DO NOTHING`,
			c.id, c.activity, c.member, c.text); err != nil {
			return 0, fmt.Errorf("seed: comment %s: %w", c.id, err)
		}
	}

	// ── Clubs ────────────────────────────────────────────────────────────────
	clubs := []struct {
		id, name, description, location string
		members                         []string
	}{
		{"clb_senopati", "Senopati Sunrise", "6am, every weekday, from the studio door.", "Senopati",
			[]string{"mem_demo", "mem_jaime", "mem_lucas", "mem_sari"}},
		{"clb_pik", "PIK Seawall Club", "Long runs along the water on Saturdays.", "Pantai Indah Kapuk",
			[]string{"mem_natalie", "mem_doris", "mem_luke"}},
	}
	for _, club := range clubs {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO training.clubs (id, name, description, location)
			VALUES ($1,$2,$3,$4) ON CONFLICT (id) DO NOTHING`,
			club.id, club.name, club.description, club.location); err != nil {
			return 0, fmt.Errorf("seed: club %s: %w", club.id, err)
		}
		for _, memberID := range club.members {
			if _, err := s.db.Exec(ctx,
				`INSERT INTO training.club_members (club_id, member_id) VALUES ($1,$2)
				 ON CONFLICT DO NOTHING`, club.id, memberID); err != nil {
				return 0, fmt.Errorf("seed: club member %s: %w", memberID, err)
			}
		}
	}

	// ── Routes ───────────────────────────────────────────────────────────────
	routePoints, err := json.Marshal(loopTrack(senopatiLat, senopatiLng, 10_000, 340, 45))
	if err != nil {
		return 0, fmt.Errorf("seed: encoding route: %w", err)
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO training.routes (id, member_id, name, points, distance_m)
		VALUES ('rte_senopati_10k', 'mem_demo', 'Senopati 10k loop', $1, 10000)
		ON CONFLICT (id) DO NOTHING`, routePoints); err != nil {
		return 0, fmt.Errorf("seed: route: %w", err)
	}

	// ── Challenges ───────────────────────────────────────────────────────────
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	challenges := []struct {
		id, name, description, kind string
		targetKm                    float64
		startsAt, endsAt            time.Time
		joined                      []string
	}{
		{"chl_100k", "100 km this month", "Run, ride or walk your way to a hundred.", "ANY", 100,
			monthStart, monthStart.AddDate(0, 1, 0),
			[]string{"mem_demo", "mem_natalie", "mem_lucas", "mem_jaime"}},
		{"chl_run50", "50 km on foot", "Running only. Treadmill counts.", "RUN", 50,
			monthStart, monthStart.AddDate(0, 1, 0),
			[]string{"mem_demo", "mem_sari"}},
	}
	for _, c := range challenges {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO engagement.challenges (id, name, description, type, target_km, starts_at, ends_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (id) DO NOTHING`,
			c.id, c.name, c.description, c.kind, c.targetKm, c.startsAt, c.endsAt); err != nil {
			return 0, fmt.Errorf("seed: challenge %s: %w", c.id, err)
		}
		for _, memberID := range c.joined {
			if _, err := s.db.Exec(ctx,
				`INSERT INTO engagement.challenge_joins (challenge_id, member_id) VALUES ($1,$2)
				 ON CONFLICT DO NOTHING`, c.id, memberID); err != nil {
				return 0, fmt.Errorf("seed: challenge join %s: %w", memberID, err)
			}
		}
	}

	// ── The race calendar ────────────────────────────────────────────────────
	races := []struct {
		id, name, country, region, city, venue, status string
		daysFromNow                                    int
	}{
		{"rce_jakarta", "HYROX Jakarta", "Indonesia", "ASIA", "Jakarta", "JIExpo Kemayoran", "REGISTRATION_OPEN", 62},
		{"rce_singapore", "HYROX Singapore", "Singapore", "ASIA", "Singapore", "Singapore Expo", "SOLD_OUT", 34},
		{"rce_bangkok", "HYROX Bangkok", "Thailand", "ASIA", "Bangkok", "IMPACT Arena", "ANNOUNCED", 140},
		{"rce_sydney", "HYROX Sydney", "Australia", "OCEANIA", "Sydney", "ICC Sydney", "REGISTRATION_OPEN", 96},
		{"rce_london", "HYROX London", "United Kingdom", "EUROPE", "London", "Olympia London", "REGISTRATION_OPEN", 120},
		{"rce_jakarta_past", "HYROX Jakarta (last season)", "Indonesia", "ASIA", "Jakarta", "JIExpo Kemayoran", "COMPLETED", -75},
	}
	for _, race := range races {
		startsAt := now.AddDate(0, 0, race.daysFromNow)
		if _, err := s.db.Exec(ctx, `
			INSERT INTO training.race_events
				(id, name, country, region, city, venue, starts_at, ends_at, registration_url, status)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT (id) DO NOTHING`,
			race.id, race.name, race.country, race.region, race.city, race.venue,
			startsAt, startsAt.AddDate(0, 0, 1), "https://hyrox.com/find-my-race/", race.status); err != nil {
			return 0, fmt.Errorf("seed: race %s: %w", race.id, err)
		}
	}

	// The demo member is training for Jakarta and raced last season.
	entries := []struct {
		id, member, race, division, status string
		goalSec, resultSec                 *int
	}{
		{"urc_demo_jakarta", "mem_demo", "rce_jakarta", "MEN_OPEN", "TRAINING", intPtr(5400), nil},
		{"urc_demo_past", "mem_demo", "rce_jakarta_past", "MEN_OPEN", "RACED", intPtr(5700), intPtr(5612)},
		{"urc_natalie_jakarta", "mem_natalie", "rce_jakarta", "WOMEN_OPEN", "TRAINING", intPtr(6000), nil},
		{"urc_lucas_singapore", "mem_lucas", "rce_singapore", "MEN_PRO", "TRAINING", nil, nil},
	}
	for _, e := range entries {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO training.user_races (id, member_id, race_event_id, division, goal_sec, status, result_sec)
			VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (id) DO NOTHING`,
			e.id, e.member, e.race, e.division, e.goalSec, e.status, e.resultSec); err != nil {
			return 0, fmt.Errorf("seed: race entry %s: %w", e.id, err)
		}
	}

	return inserted, nil
}
