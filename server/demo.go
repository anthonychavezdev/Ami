package main

import (
	"image/color"
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

// Pronouns and bio for a few demo accounts.
var demoProfiles = map[string][2]string{
	"ana":  {"she/her", "Ingeniera de software en Madrid. Me gustan los paseos largos, el café y las fotos del atardecer."},
	"luis": {"he/him", "Fútbol los domingos, código entre semana."},
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
	private                  bool // an invite-only group chat
}{
	{"Fútbol los domingos", "Partido en el parque cada domingo a las 10.", "luis", []string{"ana", "diego", "mateo"}, false},
	{"Recetas de la abuela", "Recetas caseras y fotos de lo que cocinamos.", "sofia", []string{"ana", "camila", "valentina"}, false},
	{"Senderismo", "Rutas de fin de semana por la sierra.", "javier", []string{"camila", "mateo"}, false},
	{"Cumple de Luis 🎂", "", "ana", []string{"sofia", "diego"}, true},
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

// Group chat messages, oldest first.
var demoGroupMessages = []struct {
	from, group, body string
	ago               time.Duration
}{
	{"ana", "Cumple de Luis 🎂", "Luis no puede ver este grupo 🤫 ¿Le hacemos un regalo entre todos?", 28 * hour},
	{"sofia", "Cumple de Luis 🎂", "¡Me apunto!", 27 * hour},
	{"diego", "Cumple de Luis 🎂", "Yo también. ¿Una camiseta del equipo?", 26 * hour},
	{"sofia", "Recetas de la abuela", "Esta semana toca tortilla. ¡Quiero fotos del resultado!", 9 * hour},
	{"camila", "Recetas de la abuela", "La mía quedó un poco quemada 😅", 8 * hour},
	{"luis", "Fútbol los domingos", "¿Quién trae el balón este domingo?", 3 * hour},
	{"diego", "Fútbol los domingos", "Yo lo llevo ⚽", 170 * time.Minute},
	{"ana", "Fútbol los domingos", "Yo llevo agua para todos", 160 * time.Minute},
	{"mateo", "Fútbol los domingos", "Llego 10 minutos tarde, empezad sin mí", 40 * time.Minute},
}
