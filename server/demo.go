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
	"ana":  {"she/her", "Software engineer in Madrid. I like long walks, coffee and sunset photos."},
	"luis": {"he/him", "Football on Sundays, code on weekdays."},
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
	{"Sunday football", "A match in the park every Sunday at 10.", "luis", []string{"ana", "diego", "mateo"}, false},
	{"Grandma's recipes", "Home cooking and photos of what we make.", "sofia", []string{"ana", "camila", "valentina"}, false},
	{"Hiking", "Weekend routes in the hills.", "javier", []string{"camila", "mateo"}, false},
	{"Luis's birthday 🎂", "", "ana", []string{"sofia", "diego"}, true},
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
	"demo-paella.png": {{R: 230, G: 120, B: 40, A: 255}, {R: 250, G: 210, B: 90, A: 255}},
	"demo-sunset.png": {{R: 90, G: 40, B: 130, A: 255}, {R: 250, G: 140, B: 70, A: 255}},
	"demo-park.png":   {{R: 40, G: 120, B: 70, A: 255}, {R: 150, G: 210, B: 120, A: 255}},
}

func demoImage(name string) Attachment {
	return Attachment{Type: "image", URL: "/media/" + name, Width: 1200, Height: 900}
}

const hour = time.Hour

// Oldest first: insertion order is feed order.
var demoPosts = []demoPost{
	{"luis", "", "Hello Ami! First post from the new app 🎉", 5 * 24 * hour,
		[]demoComment{{"ana", "Welcome!"}, {"sofia", "Finally 😄"}}, map[string]string{"ana": "👍", "sofia": "❤️", "diego": "🔥"}, nil},
	{"sofia", "", "Tried a new paella recipe today. It came out spectacular 🥘", 4 * 24 * hour,
		[]demoComment{{"ana", "You have to send me the recipe!"}, {"camila", "Looks amazing"}}, map[string]string{"ana": "❤️", "camila": "😍"},
		[]Attachment{demoImage("demo-paella.png")}},
	{"luis", "Sunday football", "We're playing this Sunday at 10 in the park. Who's in?", 3 * 24 * hour,
		[]demoComment{{"diego", "I'm in"}, {"ana", "Count me in"}}, map[string]string{"ana": "👍", "diego": "👍", "mateo": "🔥"}, nil},
	{"diego", "", "Finished my first **half marathon** 🏃\n\n- 21 km\n- 1 h 58 min\n- _without stopping_", 70 * hour,
		[]demoComment{{"luis", "Legend!"}}, map[string]string{"ana": "👏", "luis": "💯"}, nil},
	{"sofia", "Grandma's recipes", "Recipe of the week: my grandma's Spanish omelette.", 2 * 24 * hour,
		[]demoComment{{"valentina", "The best omelette in the world"}, {"ana", "Making it tomorrow"}}, map[string]string{"ana": "😍", "valentina": "❤️"}, nil},
	{"ana", "", "Trying out Ami with my friends. What do you think?", 40 * hour,
		[]demoComment{{"luis", "Love it"}, {"sofia", "Really easy to use"}}, map[string]string{"luis": "👍", "sofia": "🎉", "diego": "👍"}, nil},
	{"valentina", "", "New job starting Monday. Nervous but happy!", 30 * hour,
		nil, map[string]string{"sofia": "🎉", "camila": "👏"}, nil},
	{"camila", "", "Anyone have book recommendations for the summer?", 26 * hour,
		[]demoComment{{"sofia", "One Hundred Years of Solitude, always."}}, map[string]string{"sofia": "🤔"}, nil},
	{"javier", "Hiking", "New route on Saturday: 14 km with views over the valley.", 20 * hour,
		[]demoComment{{"mateo", "Signed up"}}, map[string]string{"camila": "🔥", "mateo": "👍"}, nil},
	{"luis", "", "Coffee and code ☕ Happy Monday, everyone. Today's reading is WebRTC:", 8 * hour,
		[]demoComment{{"ana", "Happy Monday!"}}, map[string]string{"ana": "❤️"},
		[]Attachment{{Type: "link", URL: "https://developer.mozilla.org/en-US/docs/Web/API/WebRTC_API"}}},
	{"diego", "Sunday football", "Two short of a full team. Bring friends.", 5 * hour,
		nil, map[string]string{"luis": "👍"}, nil},
	{"ana", "", "Can't wait for the weekend!", 2 * hour,
		nil, map[string]string{"diego": "😂", "luis": "👍"}, nil},
	{"ana", "", "", 90 * time.Minute, nil, map[string]string{"luis": "👍"}, []Attachment{demoImage("demo-park.png")}},
	{"sofia", "", "Tonight's sunset from the terrace 🌅", 45 * time.Minute,
		[]demoComment{{"ana", "Beautiful"}}, map[string]string{"ana": "😍", "luis": "❤️", "camila": "😮"},
		[]Attachment{demoImage("demo-sunset.png")}},
	{"mateo", "", "Anyone up for padel this afternoon?", 20 * time.Minute, nil, nil, nil},
}

var demoMessages = []struct {
	from, to, body, image string // image: optional demo picture attached to the message
	ago                   time.Duration
}{
	{"luis", "ana", "Hi Ana! Are you coming to football on Sunday?", "", 26 * hour},
	{"ana", "luis", "Of course! What time are we meeting?", "", 25 * hour},
	{"luis", "ana", "10 at the park. Bring water 💧", "", 25*hour - 5*time.Minute},
	{"ana", "luis", "Perfect, I'll be there 🙌", "", 24 * hour},
	{"diego", "luis", "See you on Sunday", "", 7 * hour},
	{"sofia", "ana", "I'll send you the paella recipe tonight", "", 6 * hour},
	{"sofia", "ana", "This is how the last one turned out 🥘", "demo-paella.png", 6*hour - 2*time.Minute},
	{"ana", "sofia", "Thanks! I can't wait to try it", "", 5 * hour},
	{"luis", "ana", "Shall we try a video call on Ami?", "", 30 * time.Minute},
}

// Group chat messages, oldest first.
var demoGroupMessages = []struct {
	from, group, body string
	ago               time.Duration
}{
	{"ana", "Luis's birthday 🎂", "Luis can't see this group 🤫 Shall we all chip in for a present?", 28 * hour},
	{"sofia", "Luis's birthday 🎂", "I'm in!", 27 * hour},
	{"diego", "Luis's birthday 🎂", "Me too. A team shirt?", 26 * hour},
	{"sofia", "Grandma's recipes", "This week it's the omelette. I want photos of the results!", 9 * hour},
	{"camila", "Grandma's recipes", "Mine came out a bit burnt 😅", 8 * hour},
	{"luis", "Sunday football", "Who's bringing the ball this Sunday?", 3 * hour},
	{"diego", "Sunday football", "I'll bring it ⚽", 170 * time.Minute},
	{"ana", "Sunday football", "I'll bring water for everyone", 160 * time.Minute},
	{"mateo", "Sunday football", "Running 10 minutes late, start without me", 40 * time.Minute},
}
