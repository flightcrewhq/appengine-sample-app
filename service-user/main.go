package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"

	"holosam/appengine/demo/pkg/database"
	"holosam/appengine/demo/pkg/util"
)

type Handler struct {
	db *database.DBClient
}

// Add a new record to the db, and notify all followers
func (h *Handler) publishHandler(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	var pr database.PublishRequest
	err = json.Unmarshal(body, &pr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = h.db.WriteDocument(r.Context(), &pr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (h *Handler) followHandler(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	var fr database.FollowRequest
	err = json.Unmarshal(body, &fr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = h.db.ModifyUser(r.Context(), fr.Src, func(u *database.User) {
		u.AddFollowing(fr.Dst)
	}, database.ErrNoUser)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = h.db.ModifyUser(r.Context(), fr.Dst, func(u *database.User) {
		u.AddFollower(fr.Src)
	}, database.ErrNoUser)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := database.Init(ctx)
	if err != nil {
		log.Fatalf("Failed to open db client: %v", err)
	}
	defer db.Close()

	handler := &Handler{
		db: db,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/publish", handler.publishHandler)
	mux.HandleFunc("/follow", handler.followHandler)

	server := util.NewHttpServer(mux)
	log.Fatal(server.ListenAndServe())
}
