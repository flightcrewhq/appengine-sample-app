package main

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"regexp"
	"time"

	"holosam/appengine/demo/pkg/database"
	"holosam/appengine/demo/pkg/util"
)

var (
	templates = template.Must(template.ParseFiles("land.html", "feed.html"))

	// https://github.com/gorilla/mux is the solution to this hack.
	userRegex = regexp.MustCompile(`^/user/(\w+)`)

	numSelfDocs = util.LoadEnvInt(util.EnvSelfDocs, 3)
	numFeedDocs = util.LoadEnvInt(util.EnvFeedDocs, 5)
)

type Handler struct {
	db       *database.DBClient
	client   *util.HttpClient
	project  string
	baseTmpl *BaseTmpl
}

type BaseTmpl struct {
	Headline  string
	TextColor string
}

type FeedTmpl struct {
	Headline string
	User     string
	Feed     []DocTmpl
	Self     []DocTmpl
}

type DocTmpl struct {
	Author string
	Text   string
}

func (h *Handler) baseHandler(w http.ResponseWriter, r *http.Request) {
	if err := templates.ExecuteTemplate(w, "land.html", h.baseTmpl); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// This exists because I don't know how to go directly to /user/username from
// the land.html form entry, so this takes in the user as a param and then
// redirects.
func (h *Handler) redirectHandler(w http.ResponseWriter, r *http.Request) {
	user, err := getParam(r, "user")
	if err != nil {
		h.baseTmpl.Headline = "Missing username param"
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/user/%s", user), http.StatusFound)
}

func (h *Handler) loginHandler(w http.ResponseWriter, r *http.Request, user string) {
	err := h.db.ModifyUser(r.Context(), user, func(u *database.User) {
		u.Logins += 1
		u.LastLogin = time.Now()
	}, func() (database.User, error) {
		return database.NewUser(user), nil
	})
	if err != nil {
		log.Printf("User error: %v", err)
		h.baseTmpl.Headline = "Failed to access user"
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	feedTmpl, err := h.buildFeed(r.Context(), user)
	if err != nil {
		log.Printf("Doc error: %v", err)
		h.baseTmpl.Headline = "Failed to access docs"
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	if err := templates.ExecuteTemplate(w, "feed.html", feedTmpl); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.baseTmpl.Headline = util.LoadEnvString(util.EnvHeadline, "Welcome")
}

// Should surface docs from people who they aren't following too?
func (h *Handler) buildFeed(ctx context.Context, user string) (*FeedTmpl, error) {
	feed := &FeedTmpl{
		Headline: fmt.Sprintf("Welcome %s!", user),
		User:     user,
		Feed:     make([]DocTmpl, 0),
		Self:     make([]DocTmpl, 0),
	}

	selfDocs, err := h.db.GetUserDocs(ctx, user, numSelfDocs)
	if err != nil {
		return nil, err
	}

	for _, doc := range selfDocs {
		feed.Self = append(feed.Self, DocTmpl{
			Author: doc.Author,
			Text:   doc.Text,
		})
	}

	feedDocs, err := h.db.GetFollowingDocs(ctx, user, numFeedDocs)
	if err != nil {
		return nil, err
	}

	for _, doc := range feedDocs {
		feed.Feed = append(feed.Feed, DocTmpl{
			Author: doc.Author,
			Text:   doc.Text,
		})
	}

	return feed, nil
}

func (h *Handler) publishHandler(w http.ResponseWriter, r *http.Request) {
	user, err := getParam(r, "user")
	if err != nil {
		log.Printf("Missing user param")
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	text, err := getParam(r, "text")
	if err != nil {
		log.Printf("Missing text param")
		http.Redirect(w, r, fmt.Sprintf("/user/%s", user), http.StatusTemporaryRedirect)
		return
	}

	pr := database.PublishRequest{
		User: user,
		Text: text,
	}

	_, err = h.client.Send(util.ReqOpts{
		Method:      "POST",
		Url:         fmt.Sprintf(util.UserServiceURL, h.project, "publish"),
		JsonContent: pr,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/user/%s", user), http.StatusFound)
}

func (h *Handler) followHandler(w http.ResponseWriter, r *http.Request) {
	src, srcErr := getParam(r, "src")
	dst, dstErr := getParam(r, "dst")
	if srcErr != nil || dstErr != nil {
		log.Printf("Missing src and/or dst user param")
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	fr := database.FollowRequest{
		Src: src,
		Dst: dst,
	}

	if _, err := h.client.Send(util.ReqOpts{
		Method:      "POST",
		Url:         fmt.Sprintf(util.UserServiceURL, h.project, "follow"),
		JsonContent: fr,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/user/%s", src), http.StatusFound)
}

func getParam(r *http.Request, param string) (string, error) {
	params, ok := r.URL.Query()[param]
	if !ok || len(params) == 0 {
		return "", fmt.Errorf("missing %s param", param)
	}
	return params[0], nil
}

func main() {
	log.Printf("Running version %s", util.LoadEnvString("GAE_VERSION", "[not found]"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := database.Init(ctx)
	if err != nil {
		log.Fatalf("Failed to open db client: %v", err)
	}
	defer db.Close()

	handler := &Handler{
		db:      db,
		client:  util.NewHttpClient(),
		project: util.MustLoadEnvString(util.EnvCloudProject),
		baseTmpl: &BaseTmpl{
			Headline:  util.LoadEnvString(util.EnvHeadline, "Welcome"),
			TextColor: util.LoadEnvString(util.EnvTextColor, "black"),
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handler.baseHandler)
	mux.HandleFunc("/publish", handler.publishHandler)
	mux.HandleFunc("/follow", handler.followHandler)
	mux.HandleFunc("/user", handler.redirectHandler)
	mux.HandleFunc("/user/", func(w http.ResponseWriter, r *http.Request) {
		matches := userRegex.FindStringSubmatch(r.URL.Path)
		if len(matches) == 0 {
			http.NotFound(w, r)
			return
		}
		handler.loginHandler(w, r, matches[1])
	})

	server := util.NewHttpServer(mux)
	log.Fatal(server.ListenAndServe())
}
