package main

import (
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"sync"

	flag "github.com/ogier/pflag"
)

type configuration struct {
	Bind                string
	StaticDirectory     string
	UserDataDirectory   string
	LessonDataDirectory string
	BuildDirectory      string
	LessonFileSuffix    string
	MaxUsers            int
	RobotLogBufferSize  int
}

const (
	editorFile = "editor.html"
	loginFile  = "login.html"

	loginCookieName = "login"

	srcSubDirectory = "src/main/java/com/spartronics4915/learnyouarobot"
)

var (
	users *Users
	// No synchronization needed because this should be read-only
	stockLessons       Lessons
	deployDirectoryMux sync.Mutex
	config             configuration
)

func main() {
	flag.StringVarP(&config.Bind, "bind", "b", "localhost:8000", "Address to run the webserver on.")
	flag.StringVarP(&config.StaticDirectory, "static", "s", "static", "Path to static files to serve on /.")
	flag.IntVarP(&config.MaxUsers, "max-users", "u", 20, "Maximum number of users.")
	flag.StringVarP(&config.UserDataDirectory, "user-data", "d", "users", "Path to a folder containing user data folders.")
	flag.StringVarP(&config.LessonDataDirectory, "lesson-data", "l", "lessons", "Path to a folder containing stock lessons.")
	flag.StringVar(&config.LessonFileSuffix, "lesson-suffix", ".java", "Suffix of lesson files. Anything before this will be the name of the lesson.")
	flag.IntVar(&config.RobotLogBufferSize, "robotlog-size", 1e4, "Maximum number of lines to store in the robot log buffer.")
	flag.StringVarP(&config.BuildDirectory, "build-directory", "B", "build", "Path to a folder containing build scripts, and the following directory structure:\n"+srcSubDirectory)
	flag.Parse()

	users = &Users{
		MaxUsers: config.MaxUsers,
	}

	makeStockLessons()
	loadUsers()

	http.Handle("/", indexSwitcher(http.FileServer(http.Dir(config.StaticDirectory))))
	http.HandleFunc("/api/user/login", handleLogin)
	http.HandleFunc("/api/user/lessons", handleGetUserLessons)
	http.HandleFunc("/api/lesson/get", handleGetLesson)
	http.HandleFunc("/api/lesson/save", handleSaveLesson)
	http.HandleFunc("/api/lesson/deploy", handleDeployLesson)
	http.HandleFunc("/api/lesson/deploy/queue", handleGetDeployQueue)

	log.Println("Listening on", config.Bind)
	log.Panicln(http.ListenAndServe(config.Bind, nil))
}

func loadUsers() {
	userDataDirs, err := ioutil.ReadDir(config.UserDataDirectory)
	if err != nil {
		log.Panic(err)
	}

	for _, file := range userDataDirs {
		if !file.IsDir() {
			log.Println("Skipping non-directory file", file.Name(), "in user data directory.")
			continue
		}

		_, err := users.Add(file.Name())
		if err != nil {
			log.Println("Couldn't load user from directory", file.Name()+".")
		}
	}

	log.Println("Loaded", users.NumUsers(), "preexisting users.")
}

func makeStockLessons() {
	stockLessons = NewLessons()

	stockLessonFiles, err := ioutil.ReadDir(config.LessonDataDirectory)
	if err != nil {
		log.Panicln(err.Error())
	}
	for _, file := range stockLessonFiles {
		fileName := file.Name()
		if !strings.HasSuffix(fileName, config.LessonFileSuffix) {
			continue
		}

		lesson, err := NewLesson(fileName, config.LessonDataDirectory)
		if err != nil {
			log.Panicln(err.Error())
		}
		lesson.Modified = false

		stockLessons[lesson.Name] = lesson
	}
}

func indexSwitcher(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			h.ServeHTTP(w, r)
			return
		}

		_, err := GetCurrentUser(r)
		if err != nil {
			r.URL.Path = loginFile
		} else {
			r.URL.Path = editorFile
		}
		h.ServeHTTP(w, r)
	})
}
