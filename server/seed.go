package main

import (
	"log"
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
}

const hour = time.Hour

// Oldest first: insertion order is feed order.
var demoPosts = []demoPost{
	{"luis", "", "¡Hola Ami! Primer post desde la app nueva 🎉", 5 * 24 * hour,
		[]demoComment{{"ana", "¡Bienvenido!"}, {"sofia", "Por fin 😄"}}, map[string]string{"ana": "👍", "sofia": "❤️", "diego": "🔥"}},
	{"sofia", "", "Hoy probé una receta nueva de paella. Quedó espectacular 🥘", 4 * 24 * hour,
		[]demoComment{{"ana", "¡Tienes que pasarme la receta!"}, {"camila", "Se ve increíble"}}, map[string]string{"ana": "❤️", "camila": "😍"}},
	{"luis", "Fútbol los domingos", "Este domingo jugamos a las 10 en el parque. ¿Quién se apunta?", 3 * 24 * hour,
		[]demoComment{{"diego", "Yo voy"}, {"ana", "Cuenta conmigo"}}, map[string]string{"ana": "👍", "diego": "👍", "mateo": "🔥"}},
	{"diego", "", "Terminé mi primera media maratón 🏃", 70 * hour,
		[]demoComment{{"luis", "¡Crack!"}}, map[string]string{"ana": "👏", "luis": "💯"}},
	{"sofia", "Recetas de la abuela", "Receta de la semana: la tortilla de patatas de mi abuela.", 2 * 24 * hour,
		[]demoComment{{"valentina", "La mejor tortilla del mundo"}, {"ana", "Mañana la hago"}}, map[string]string{"ana": "😍", "valentina": "❤️"}},
	{"ana", "", "Probando Ami con mis amigos. ¿Qué os parece?", 40 * hour,
		[]demoComment{{"luis", "Me encanta"}, {"sofia", "Muy fácil de usar"}}, map[string]string{"luis": "👍", "sofia": "🎉", "diego": "👍"}},
	{"valentina", "", "Nuevo trabajo a partir del lunes. ¡Nerviosa pero feliz!", 30 * hour,
		nil, map[string]string{"sofia": "🎉", "camila": "👏"}},
	{"camila", "", "¿Alguien tiene recomendaciones de libros para el verano?", 26 * hour,
		[]demoComment{{"sofia", "Cien años de soledad, siempre."}}, map[string]string{"sofia": "🤔"}},
	{"javier", "Senderismo", "Ruta nueva el sábado: 14 km con vistas al valle.", 20 * hour,
		[]demoComment{{"mateo", "Apuntado"}}, map[string]string{"camila": "🔥", "mateo": "👍"}},
	{"luis", "", "Café y código ☕ Buen lunes a todos.", 8 * hour,
		[]demoComment{{"ana", "¡Buen lunes!"}}, map[string]string{"ana": "❤️"}},
	{"diego", "Fútbol los domingos", "Faltan dos para completar el equipo. Traed amigos.", 5 * hour,
		nil, map[string]string{"luis": "👍"}},
	{"ana", "", "¡Qué ganas de que llegue el fin de semana!", 2 * hour,
		nil, map[string]string{"diego": "😂", "luis": "👍"}},
	{"sofia", "", "El atardecer de hoy desde la terraza 🌅", 45 * time.Minute,
		[]demoComment{{"ana", "Precioso"}}, map[string]string{"ana": "😍", "luis": "❤️", "camila": "😮"}},
	{"mateo", "", "¿Alguien para jugar al pádel esta tarde?", 20 * time.Minute, nil, nil},
}

var demoMessages = []struct {
	from, to, body string
	ago            time.Duration
}{
	{"luis", "ana", "¡Hola Ana! ¿Vienes el domingo al fútbol?", 26 * hour},
	{"ana", "luis", "¡Claro! ¿A qué hora quedamos?", 25 * hour},
	{"luis", "ana", "A las 10 en el parque. Trae agua 💧", 25*hour - 5*time.Minute},
	{"ana", "luis", "Perfecto, ahí estaré 🙌", 24 * hour},
	{"diego", "luis", "Nos vemos el domingo", 7 * hour},
	{"sofia", "ana", "Te mando la receta de la paella esta noche", 6 * hour},
	{"ana", "sofia", "¡Gracias! Tengo muchas ganas de probarla", 5 * hour},
	{"luis", "ana", "¿Probamos una videollamada en Ami?", 30 * time.Minute},
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
		res, err := tx.Exec(`INSERT INTO users(username, display_name, password_hash, salt, created_at) VALUES (?, ?, ?, ?, ?)`,
			u.username, u.name, hashPassword(demoPassword, salt), salt, ts(time.Duration(14-i)*24*hour))
		if err != nil {
			return err
		}
		users[u.username], _ = res.LastInsertId()
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
	for _, p := range demoPosts {
		var groupID any
		if p.group != "" {
			groupID = groups[p.group]
		}
		res, err := tx.Exec(`INSERT INTO posts(author_id, group_id, body, created_at) VALUES (?, ?, ?, ?)`,
			users[p.author], groupID, p.body, ts(p.ago))
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
		if _, err := tx.Exec(`INSERT INTO messages(sender_id, recipient_id, body, created_at) VALUES (?, ?, ?, ?)`,
			users[m.from], users[m.to], m.body, ts(m.ago)); err != nil {
			return err
		}
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
