package main

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Demo content for development mode (-dev). It is only inserted into an empty
// database; every demo account uses demoPassword. Log in as "ana" to see
// every friendship state, group role and conversation.
const demoPassword = "amigo-demo"

var demoUsers = []struct{ username, name string }{
	{"ana", "Ana García"},
	{"luis", "Luis Martínez"},
	{"sofia", "Sofía López"},
	{"diego", "Diego Hernández"},
	{"valentina", "Valentina Pérez"},
	{"mateo", "Mateo Rodríguez"},
	{"camila", "Camila Torres"},
	{"javier", "Javier Ramírez"},
}

var demoFriendships = []struct {
	from, to string
	accepted bool
}{
	{"ana", "luis", true},
	{"sofia", "ana", true},
	{"ana", "diego", true},
	{"valentina", "ana", false}, // incoming request for ana
	{"ana", "mateo", false},     // request sent by ana
	{"luis", "sofia", true},
	{"luis", "diego", true},
	{"sofia", "camila", true},
	{"javier", "camila", true},
	{"mateo", "javier", true},
}

var demoGroups = []struct {
	name, description, owner string
	members                  []string
}{
	{"Fútbol los domingos", "Partido en el parque cada domingo a las 10.", "luis", []string{"ana", "diego", "mateo"}},
	{"Recetas de la abuela", "Recetas caseras y fotos de lo que cocinamos.", "sofia", []string{"ana", "camila", "valentina"}},
	{"Senderismo", "Rutas de fin de semana por la sierra.", "javier", []string{"camila", "mateo"}},
}

type demoComment struct{ author, body string }

type demoPost struct {
	author, group, body string
	ago                 time.Duration
	comments            []demoComment
	reactions           map[string]string // username -> emoji
	attachments         []Attachment
}

// Demo pictures are gradients generated at seed time; the names below are
// referenced by demoPosts.
var demoImages = map[string][2]color.RGBA{
	"demo-paella.png":    {{R: 230, G: 120, B: 40, A: 255}, {R: 250, G: 210, B: 90, A: 255}},
	"demo-atardecer.png": {{R: 90, G: 40, B: 130, A: 255}, {R: 250, G: 140, B: 70, A: 255}},
	"demo-parque.png":    {{R: 40, G: 120, B: 70, A: 255}, {R: 150, G: 210, B: 120, A: 255}},
}

func demoImage(name string) Attachment {
	return Attachment{Type: "image", URL: "/media/" + name, Width: 1200, Height: 900}
}

const hour = time.Hour

// Oldest first: insertion order is feed order.
var demoPosts = []demoPost{
	{"luis", "", "¡Hola Ami! Primer post desde la app nueva 🎉", 5 * 24 * hour,
		[]demoComment{{"ana", "¡Bienvenido!"}, {"sofia", "Por fin 😄"}}, map[string]string{"ana": "👍", "sofia": "❤️", "diego": "🔥"}, nil},
	{"sofia", "", "Hoy probé una receta nueva de paella. Quedó espectacular 🥘", 4 * 24 * hour,
		[]demoComment{{"ana", "¡Tienes que pasarme la receta!"}, {"camila", "Se ve increíble"}}, map[string]string{"ana": "❤️", "camila": "😍"},
		[]Attachment{demoImage("demo-paella.png")}},
	{"luis", "Fútbol los domingos", "Este domingo jugamos a las 10 en el parque. ¿Quién se apunta?", 3 * 24 * hour,
		[]demoComment{{"diego", "Yo voy"}, {"ana", "Cuenta conmigo"}}, map[string]string{"ana": "👍", "diego": "👍", "mateo": "🔥"}, nil},
	{"diego", "", "Terminé mi primera **media maratón** 🏃\n\n- 21 km\n- 1 h 58 min\n- _sin parar_", 70 * hour,
		[]demoComment{{"luis", "¡Crack!"}}, map[string]string{"ana": "👏", "luis": "💯"}, nil},
	{"sofia", "Recetas de la abuela", "Receta de la semana: la tortilla de patatas de mi abuela.", 2 * 24 * hour,
		[]demoComment{{"valentina", "La mejor tortilla del mundo"}, {"ana", "Mañana la hago"}}, map[string]string{"ana": "😍", "valentina": "❤️"}, nil},
	{"ana", "", "Probando Ami con mis amigos. ¿Qué os parece?", 40 * hour,
		[]demoComment{{"luis", "Me encanta"}, {"sofia", "Muy fácil de usar"}}, map[string]string{"luis": "👍", "sofia": "🎉", "diego": "👍"}, nil},
	{"valentina", "", "Nuevo trabajo a partir del lunes. ¡Nerviosa pero feliz!", 30 * hour,
		nil, map[string]string{"sofia": "🎉", "camila": "👏"}, nil},
	{"camila", "", "¿Alguien tiene recomendaciones de libros para el verano?", 26 * hour,
		[]demoComment{{"sofia", "Cien años de soledad, siempre."}}, map[string]string{"sofia": "🤔"}, nil},
	{"javier", "Senderismo", "Ruta nueva el sábado: 14 km con vistas al valle.", 20 * hour,
		[]demoComment{{"mateo", "Apuntado"}}, map[string]string{"camila": "🔥", "mateo": "👍"}, nil},
	{"luis", "", "Café y código ☕ Buen lunes a todos. Hoy toca leer sobre WebRTC:", 8 * hour,
		[]demoComment{{"ana", "¡Buen lunes!"}}, map[string]string{"ana": "❤️"},
		[]Attachment{{Type: "link", URL: "https://developer.mozilla.org/en-US/docs/Web/API/WebRTC_API"}}},
	{"diego", "Fútbol los domingos", "Faltan dos para completar el equipo. Traed amigos.", 5 * hour,
		nil, map[string]string{"luis": "👍"}, nil},
	{"ana", "", "¡Qué ganas de que llegue el fin de semana!", 2 * hour,
		nil, map[string]string{"diego": "😂", "luis": "👍"}, nil},
	{"ana", "", "", 90 * time.Minute, nil, map[string]string{"luis": "👍"}, []Attachment{demoImage("demo-parque.png")}},
	{"sofia", "", "El atardecer de hoy desde la terraza 🌅", 45 * time.Minute,
		[]demoComment{{"ana", "Precioso"}}, map[string]string{"ana": "😍", "luis": "❤️", "camila": "😮"},
		[]Attachment{demoImage("demo-atardecer.png")}},
	{"mateo", "", "¿Alguien para jugar al pádel esta tarde?", 20 * time.Minute, nil, nil, nil},
}

// Pronouns and bio for a few demo accounts.
var demoProfiles = map[string][2]string{
	"ana":  {"she/her", "Ingeniera de software en Madrid. Me gustan los paseos largos, el café y las fotos del atardecer."},
	"luis": {"he/him", "Fútbol los domingos, código entre semana."},
}

var demoMessages = []struct {
	from, to, body, image string // image: optional demo picture attached to the message
	ago                   time.Duration
}{
	{"luis", "ana", "¡Hola Ana! ¿Vienes el domingo al fútbol?", "", 26 * hour},
	{"ana", "luis", "¡Claro! ¿A qué hora quedamos?", "", 25 * hour},
	{"luis", "ana", "A las 10 en el parque. Trae agua 💧", "", 25*hour - 5*time.Minute},
	{"ana", "luis", "Perfecto, ahí estaré 🙌", "", 24 * hour},
	{"diego", "luis", "Nos vemos el domingo", "", 7 * hour},
	{"sofia", "ana", "Te mando la receta de la paella esta noche", "", 6 * hour},
	{"sofia", "ana", "Así quedó la última que hice 🥘", "demo-paella.png", 6*hour - 2*time.Minute},
	{"ana", "sofia", "¡Gracias! Tengo muchas ganas de probarla", "", 5 * hour},
	{"luis", "ana", "¿Probamos una videollamada en Ami?", "", 30 * time.Minute},
}

// seedIfEmpty inserts the demo content when the database has no users yet.
func (s *server) seedIfEmpty() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	base := now()
	ts := func(ago time.Duration) string { return base.Add(-ago).Format(time.RFC3339Nano) }

	users := map[string]int64{}
	for i, u := range demoUsers {
		salt := randomBytes(16)
		res, err := tx.Exec(`INSERT INTO users(username, display_name, password_hash, salt, password_iterations, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			u.username, u.name, hashPassword(demoPassword, salt, pbkdf2Iterations), salt, pbkdf2Iterations, ts(time.Duration(14-i)*24*hour))
		if err != nil {
			return err
		}
		users[u.username], _ = res.LastInsertId()
		if p, ok := demoProfiles[u.username]; ok {
			if _, err := tx.Exec(`UPDATE users SET pronouns = ?, bio = ? WHERE id = ?`, p[0], p[1], users[u.username]); err != nil {
				return err
			}
		}
	}
	for _, f := range demoFriendships {
		status := "pending"
		if f.accepted {
			status = "accepted"
		}
		if _, err := tx.Exec(`INSERT INTO friendships(from_id, to_id, status, created_at) VALUES (?, ?, ?, ?)`,
			users[f.from], users[f.to], status, ts(6*24*hour)); err != nil {
			return err
		}
	}
	groups := map[string]int64{}
	for _, g := range demoGroups {
		res, err := tx.Exec(`INSERT INTO groups(name, description, owner_id, created_at) VALUES (?, ?, ?, ?)`,
			g.name, g.description, users[g.owner], ts(6*24*hour))
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		groups[g.name] = id
		if _, err := tx.Exec(`INSERT INTO group_members(group_id, user_id, role, joined_at) VALUES (?, ?, 'admin', ?)`, id, users[g.owner], ts(6*24*hour)); err != nil {
			return err
		}
		for _, m := range g.members {
			if _, err := tx.Exec(`INSERT INTO group_members(group_id, user_id, role, joined_at) VALUES (?, ?, 'member', ?)`, id, users[m], ts(5*24*hour)); err != nil {
				return err
			}
		}
	}
	for name, c := range demoImages {
		if err := writeGradient(filepath.Join(s.mediaDir, name), c[0], c[1]); err != nil {
			return err
		}
	}
	for _, p := range demoPosts {
		var groupID any
		if p.group != "" {
			groupID = groups[p.group]
		}
		atts := p.attachments
		if atts == nil {
			atts = []Attachment{}
		}
		attachments, _ := json.Marshal(atts)
		res, err := tx.Exec(`INSERT INTO posts(author_id, group_id, body, attachments, created_at) VALUES (?, ?, ?, ?, ?)`,
			users[p.author], groupID, p.body, string(attachments), ts(p.ago))
		if err != nil {
			return err
		}
		postID, _ := res.LastInsertId()
		for i, c := range p.comments {
			if _, err := tx.Exec(`INSERT INTO comments(post_id, author_id, body, created_at) VALUES (?, ?, ?, ?)`,
				postID, users[c.author], c.body, ts(p.ago-time.Duration(i+1)*7*time.Minute)); err != nil {
				return err
			}
		}
		for user, emoji := range p.reactions {
			if _, err := tx.Exec(`INSERT INTO reactions(post_id, user_id, emoji) VALUES (?, ?, ?)`, postID, users[user], emoji); err != nil {
				return err
			}
		}
	}
	for _, m := range demoMessages {
		atts := "[]"
		if m.image != "" {
			b, _ := json.Marshal([]Attachment{demoImage(m.image)})
			atts = string(b)
		}
		if _, err := tx.Exec(`INSERT INTO messages(sender_id, recipient_id, body, attachments, created_at) VALUES (?, ?, ?, ?, ?)`,
			users[m.from], users[m.to], m.body, atts, ts(m.ago)); err != nil {
			return err
		}
	}
	// Everything is read except Luis's latest message to Ana, so her Messages tab shows a badge.
	if _, err := tx.Exec(`INSERT INTO conversation_reads(user_id, peer_id, last_read_id)
		SELECT recipient_id, sender_id, MAX(id) FROM messages GROUP BY recipient_id, sender_id`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE conversation_reads SET last_read_id =
		(SELECT id FROM messages WHERE sender_id = ?2 AND recipient_id = ?1 ORDER BY id DESC LIMIT 1 OFFSET 1)
		WHERE user_id = ?1 AND peer_id = ?2`, users["ana"], users["luis"]); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	names := make([]string, 0, len(demoUsers))
	for _, u := range demoUsers {
		names = append(names, u.username)
	}
	log.Printf("dev: seeded demo data; log in as %s (password %q)", strings.Join(names, ", "), demoPassword)
	return nil
}

// writeGradient renders a vertical gradient PNG (1200x900) at path.
func writeGradient(path string, from, to color.RGBA) error {
	const w, h = 1200, 900
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		t := float64(y) / float64(h-1)
		c := color.RGBA{
			R: uint8(float64(from.R)*(1-t) + float64(to.R)*t),
			G: uint8(float64(from.G)*(1-t) + float64(to.G)*t),
			B: uint8(float64(from.B)*(1-t) + float64(to.B)*t),
			A: 255,
		}
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
