package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/schollz/progressbar/v3"
)

func parseArgs() ([]string, time.Time, bool, string, string) {
	circleToken := os.Getenv("CIRCLECI_TOKEN")
	if circleToken == "" {
		log.Error().Msg("Must set CIRCLECI_TOKEN environment variable")
		return nil, time.Time{}, false, "", ""
	}
	athenianToken := os.Getenv("ATHENIAN_TOKEN")
	if athenianToken == "" {
		log.Error().Msg("Must set ATHENIAN_TOKEN environment variable")
		return nil, time.Time{}, false, "", ""
	}
	var sinceStr string
	flag.StringVar(&sinceStr, "s", time.Now().AddDate(-1, -3, 0).Format("2006-01-02"),
		"Load pipelines started after this date.")
	dryRun := flag.Bool("dry-run", false, "Print release notifications instead of sending")
	flag.Parse()
	repos := flag.Args()
	for _, repo := range repos {
		if strings.HasPrefix(repo, "-") {
			log.Error().Msgf("\"%v\" is not a repository name, flags must go first", repo)
			return nil, time.Time{}, false, "", ""
		}
	}
	since, err := time.Parse("2006-01-02", sinceStr)
	if err != nil {
		log.Error().Msgf("Invalid date: %v (must be YYYY-MM-DD)", sinceStr)
		return nil, time.Time{}, false, "", ""
	}
	return repos, since, *dryRun, circleToken, athenianToken
}

func makeCircleAPIRequest(endpoint, token string) ([]byte, int) {
	var body []byte
	var remaining int
	attempts := 10
	for attempt := 1; attempt <= attempts && len(body) == 0; attempt++ {
		body, remaining = func() ([]byte, int) {
			req, err := http.NewRequest(
				http.MethodGet,
				"https://circleci.com/api/v2/"+endpoint,
				nil,
			)
			if err != nil {
				log.Fatal().Msgf("[%d/%d] error creating HTTP request: %v", attempt, attempts, err)
			}
			req.Header.Add("Circle-Token", token)
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				log.Error().Msgf("[%d/%d] error sending HTTP request: %v", attempt, attempts, err)
				return nil, 0
			}
			defer res.Body.Close()
			remaining, err := strconv.Atoi(res.Header.Get("X-Ratelimit-Remaining"))
			if err != nil {
				log.Error().Msgf("[%d/%d] error reading the rate limit: %v", attempt, attempts, err)
				return nil, 0
			}
			body, err := ioutil.ReadAll(res.Body)
			if remaining == 0 {
				log.Warn().Msgf("[%d/%d] drained the rate limit, waiting 60s", attempt, attempts)
				time.Sleep(time.Minute)
				return nil, 0
			}
			if err != nil {
				log.Error().Msgf("[%d/%d] error reading HTTP response: %v", attempt, attempts, err)
				body = nil
			}
			return body, remaining
		}()
	}
	return body, remaining
}

type Pipeline struct {
	CreatedAt string `json:"created_at"`
	State     string `json:"state"`
	Trigger   struct {
		Actor struct {
			Login string `json:"login"`
		} `json:"actor"`
	} `json:"trigger"`
	VCS struct {
		Revision string `json:"revision"`
	} `json:"vcs"`
}

type Pipelines struct {
	NextPageToken string     `json:"next_page_token"`
	Items         []Pipeline `json:"items"`
}

type Release struct {
	PublishedAt time.Time `json:"published_at"`
	Author      string    `json:"author"`
	Commit      string    `json:"commit"`
	Repository  string    `json:"repository"`
	Name        string    `json:"name"`
}

func loadPipelines(repo, branch string, since time.Time, token string) []Release {
	var pageToken string
	lastCreatedAt := since
	var pipelines Pipelines
	var releases []Release
	if branch != "" {
		branch = fmt.Sprintf("branch=%s", branch)
	}
	bar := progressbar.Default(-1)
	defer bar.Finish()
	for lastCreatedAt.Sub(since) >= 0 {
		if pageToken != "" {
			pageToken = fmt.Sprintf("page-token=%s", pageToken)
		}
		query := strings.Trim(strings.Join([]string{branch, pageToken}, "&"), "&")
		if query != "" {
			query = "?" + query
		}
		response, rateLimit := makeCircleAPIRequest(
			fmt.Sprintf("project/gh/%s/pipeline%s", repo, query),
			token)
		bar.Describe(fmt.Sprintf("Loaded %d pipelines since %s [rate limit %d]",
			len(releases), lastCreatedAt.Format("2006-01-02"), rateLimit))
		if err := json.Unmarshal(response, &pipelines); err != nil {
			return nil
		}
		pageToken = pipelines.NextPageToken
		for _, pipeline := range pipelines.Items {
			var err error
			createdAt, err := time.Parse("2006-01-02T15:04:05Z", pipeline.CreatedAt)
			if err != nil {
				log.Error().Msgf("Invalid datetime format: %v", pipeline.CreatedAt)
				return nil
			}
			releases = append(releases, Release{
				PublishedAt: createdAt,
				Author:      "github.com/" + pipeline.Trigger.Actor.Login,
				Commit:      pipeline.VCS.Revision,
				Repository:  "github.com/" + repo,
				Name: fmt.Sprintf(
					"%s-%s", createdAt.Format("2006-01-02"), pipeline.VCS.Revision[:7]),
			})
		}
		lastCreatedAt = releases[len(releases)-1].PublishedAt
	}
	return releases
}

func sendReleasesBatch(releases []Release, token string, dryRun bool) error {
	data, err := json.Marshal(releases)
	if err != nil {
		return err
	}
	if dryRun {
		fmt.Println(string(data))
		return nil
	}
	req, err := http.NewRequest(
		http.MethodPost,
		"https://api.athenian.co/v1/events/releases",
		bytes.NewBuffer(data),
	)
	if err != nil {
		log.Error().Msgf("error creating HTTP request: %v", err)
		return err
	}
	req.Header.Add("X-API-Key", token)
	req.Header.Add("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	var feedback []byte
	if err == nil {
		defer res.Body.Close()
		feedback, err = ioutil.ReadAll(res.Body)
	} else {
		log.Error().Msgf("error sending Athenian API request: %v", err)
		return err
	}
	if res.StatusCode != 200 {
		log.Error().Msgf("Athenian API returned %s:\n%s", res.Status, string(feedback))
		return fmt.Errorf("server returned %s", res.Status)
	}
	return err
}

func sendReleases(releases []Release, token string, dryRun bool) {
	bar := progressbar.Default(int64(len(releases)))
	defer bar.Finish()
	buffer := make([]Release, 0, 100)
	for _, release := range releases {
		if len(buffer) == cap(buffer) {
			if sendReleasesBatch(buffer, token, dryRun) != nil {
				continue
			}
			_ = bar.Add(len(buffer))
			buffer = buffer[:0]
		}
		buffer = append(buffer, release)
	}
	if len(buffer) > 0 {
		for sendReleasesBatch(buffer, token, dryRun) != nil {
		}
	}
}

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	repos, since, dryRun, circleToken, athenianToken := parseArgs()
	if len(repos) == 0 {
		os.Exit(1)
	}
	log.Info().Msgf("Loading the pipelines for %d repositories", len(repos))
	var releases []Release
	for _, repo := range repos {
		parts := strings.Split(repo, "@")
		var repoName, branch string
		if len(parts) == 1 {
			repoName = repo
		} else {
			repoName, branch = parts[0], parts[1]
		}
		releases = append(releases, loadPipelines(repoName, branch, since, circleToken)...)
	}
	log.Info().Msgf("Sending %d release notifications to Athenian", len(releases))
	sendReleases(releases, athenianToken, dryRun)
}
