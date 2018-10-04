package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const apiEndpoint = "https://dixie.instructure.com"

var authHeader string
var directory string
var dry bool
var normalize bool

func main() {
	token := os.Getenv("CANVAS_TOKEN")
	if token == "" {
		log.Fatalf("Must set CANVAS_TOKEN environment variable")
	}
	authHeader = fmt.Sprintf("Bearer %s", token)

	var courseID int

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] [filter terms]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "If filter terms are supplied, a submission will only be downloaded\n")
		fmt.Fprintf(os.Stderr, "if every term is satisfied. A term is satisfied if it is a\n")
		fmt.Fprintf(os.Stderr, "case-insensitive substring match for any of the following:\n")
		fmt.Fprintf(os.Stderr, "  * Assignment name\n")
		fmt.Fprintf(os.Stderr, "  * Assignment description\n")
		fmt.Fprintf(os.Stderr, "  * Student login\n")
		fmt.Fprintf(os.Stderr, "  * Student name\n")
		fmt.Fprintf(os.Stderr, "  * Student email\n\n")
		fmt.Fprintf(os.Stderr, "Recognized flags:\n")
		flag.PrintDefaults()
	}
	flag.IntVar(&courseID, "course", 0, "course ID (required)")
	flag.StringVar(&directory, "dir", ".", "directory to download into")
	flag.BoolVar(&dry, "dry", false, "dry run")
	flag.BoolVar(&normalize, "despace", true, "convert spaces in file names to underscores")
	flag.Parse()

	var terms []string
	for _, raw := range flag.Args() {
		terms = append(terms, strings.ToLower(raw))
	}

	switch {
	case courseID > 0:
		syncCourse(courseID, terms)

	default:
		flag.Usage()
	}
}

type Course struct {
	ID         int       `json:"id"`
	Name       string    `json:"name"`
	CourseCode string    `json:"course_code"`
	StartAt    time.Time `json:"start_at"`
	EndAt      time.Time `json:"end_at"`
}

type Assignment struct {
	ID                      int       `json:"id"`
	Name                    string    `json:"name"`
	Description             string    `json:"description"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
	DueAt                   time.Time `json:"due_at"`
	LockAt                  time.Time `json:"lock_at"`
	UnlockAt                time.Time `json:"unlock_at"`
	PointsPossible          float64   `json:"points_possible"`
	SubmissionTypes         []string  `json:"submission_types"`
	HasSubmittedSubmissions bool      `json:"has_submitted_submissions"`
	Published               bool      `json:"published"`
}

type Submission struct {
	ID             int           `json:"id"`
	SubmittedAt    time.Time     `json:"submitted_at"`
	UserID         int           `json:"user_id"`
	User           User          `json:"user"`
	SubmissionType string        `json:"submission_type"`
	Attachments    []*Attachment `json:"attachments"`
}

type Attachment struct {
	ID          int       `json:"id"`
	DisplayName string    `json:"display_name"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content-type"`
	URL         string    `json:"url"`
	Size        int       `json:"size"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ModifiedAt  time.Time `json:"modified_at"`
}

type User struct {
	LoginID   string `json:"login_id"`
	Name      string `json:"name"`
	ShortName string `json:"short_name"`
	Email     string `json:"email"`
}

func normalizeName(ugly string) string {
	if !normalize {
		return ugly
	}
	return strings.Replace(ugly, " ", "_", -1)
}

func syncCourse(courseID int, terms []string) {
	now := time.Now()
	currentDirs := make(map[string]bool)
	currentFiles := make(map[string]bool)

	// fetch the course
	courseURL := fmt.Sprintf("%s/api/v1/courses/%d", apiEndpoint, courseID)
	course := Course{}
	mustFetch(courseURL, &course)

	switch {
	case now.Before(course.StartAt):
		fmt.Printf("%s (starts in %v)\n", course.CourseCode, course.StartAt.Sub(now))
	case now.Before(course.EndAt):
		fmt.Printf("%s (ends in %v)\n", course.Name, course.EndAt.Sub(now))
	default:
		fmt.Printf("%s (course has ended)\n", course.Name)
	}

	// get all the assignments for this course
	assignmentListURL := fmt.Sprintf("%s/api/v1/courses/%d/assignments?per_page=1000", apiEndpoint, courseID)
	var assignments []*Assignment
	mustFetch(assignmentListURL, &assignments)

	for _, asst := range assignments {
		msg := fmt.Sprintf("==> %s", asst.Name)
		if !asst.Published {
			msg += " (unpublished)"
		}

		onlineUpload := false
		for _, elt := range asst.SubmissionTypes {
			if elt == "online_upload" {
				onlineUpload = true
				break
			}
		}
		if !onlineUpload {
			msg += " (online uploads not enabled)"
			fmt.Println(msg)
			continue
		}
		if !asst.HasSubmittedSubmissions {
			msg += " (online uploads enabled, but no submissions)"
			fmt.Println(msg)
			continue
		}
		fmt.Println(msg)

		// get the list of student submissions
		var submissions []*Submission
		submissionsURL := fmt.Sprintf("%s/api/v1/courses/%d/assignments/%d/submissions?include[]=user&per_page=1000", apiEndpoint, courseID, asst.ID)
		mustFetch(submissionsURL, &submissions)

	submissionLoop:
		for _, submission := range submissions {
			if len(terms) > 0 {
				// skip this submission if it does not match all of the filter terms
				desc := strings.ToLower(asst.Name + "," +
					asst.Description + "," +
					submission.User.LoginID + "," +
					submission.User.Name + "," +
					submission.User.ShortName + "," +
					submission.User.Email)
				for _, term := range terms {
					if !strings.Contains(desc, term) {
						fmt.Printf("    %s:%s does not match filter term %q\n", submission.User.LoginID, submission.User.Name, term)
						continue submissionLoop
					}
				}
			}

			switch submission.SubmissionType {
			case "":
				fmt.Printf("    %s:%s has no submission\n", submission.User.LoginID, submission.User.Name)
			case "online_upload":
				fmt.Printf("    %s:%s submitted at %v\n", submission.User.LoginID, submission.User.Name, submission.SubmittedAt.Local())

				// process each file in the submission
				for _, attachment := range submission.Attachments {
					// get the names of the directories and file
					asstDir := filepath.Join(directory, normalizeName(course.CourseCode), normalizeName(asst.Name))
					userDir := filepath.Join(asstDir, normalizeName(fmt.Sprintf("%s:%s", submission.User.LoginID, submission.User.Name)))
					path := filepath.Join(userDir, normalizeName(attachment.Filename))

					// these are current names, so do not delete them later
					currentDirs[asstDir] = true
					currentDirs[userDir] = true
					currentFiles[path] = true

					// see if the file already exists and is unchanged
					info, err := os.Stat(path)
					if err == nil && info.Size() == int64(attachment.Size) && info.ModTime().Round(time.Second).Equal(attachment.ModifiedAt.Round(time.Second)) {
						fmt.Printf("        (unchanged) %s\n", attachment.Filename)
						continue
					}

					// if we were asked to not actually change anything, then do not download it
					if dry {
						fmt.Printf("        need to download %s (size %d) modified at %v\n", attachment.Filename, attachment.Size, attachment.ModifiedAt.Local())
						continue
					}

					// download the file
					fmt.Printf("        downloading %s (size %d) modified at %v\n", attachment.Filename, attachment.Size, attachment.ModifiedAt.Local())
					var data []byte
					mustFetch(attachment.URL, &data)
					if len(data) != attachment.Size {
						log.Fatalf("download size of %d did not match expected size of %d", len(data), attachment.Size)
					}

					// make sure the directory exists
					if err := os.MkdirAll(userDir, 0755); err != nil {
						log.Fatalf("creating directory %s: %v", userDir, err)
					}

					// save the file
					if err := ioutil.WriteFile(path, data, 0644); err != nil {
						log.Fatalf("saving file: %v", err)
					}

					// sync the timestamp
					if err := os.Chtimes(path, attachment.ModifiedAt, attachment.ModifiedAt); err != nil {
						log.Fatalf("setting timestamp: %v", err)
					}
				}
			default:
				fmt.Printf("    %s:%s has submission of type %s (skipping))\n", submission.User.LoginID, submission.User.Name, submission.SubmissionType)
			}
		}
	}

	// delete files and directories that were not synced
	courseRoot := filepath.Join(directory, normalizeName(course.CourseCode))
	currentDirs[courseRoot] = true
	var filesToDelete []string
	var dirsToDelete []string
	err := filepath.Walk(courseRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && !currentDirs[path] {
			dirsToDelete = append(dirsToDelete, path)
		} else if !info.IsDir() && !currentFiles[path] {
			filesToDelete = append(filesToDelete, path)
		}
		return nil
	})
	if err != nil {
		log.Fatalf("walking directory looking for files to delete: %v", err)
	}
	for _, path := range filesToDelete {
		if dry {
			fmt.Printf("need to delete file %s\n", path)
		} else {
			fmt.Printf("deleting file %s\n", path)
			if err := os.Remove(path); err != nil {
				log.Fatalf("removing %s: %v", path, err)
			}
		}
	}

	// delete directories after deleting their subdirectories
	sort.Strings(dirsToDelete)
	for i := len(dirsToDelete) - 1; i >= 0; i-- {
		path := dirsToDelete[i]
		if dry {
			fmt.Printf("need to delete directory %s\n", path)
		} else {
			fmt.Printf("deleting dir %s\n", path)
			if err := os.Remove(path); err != nil {
				log.Fatalf("removing %s: %v", path, err)
			}
		}
	}
}

func mustFetch(targetURL string, elt interface{}) {
	token := os.Getenv("CANVAS_TOKEN")
	if token == "" {
		log.Fatalf("Must set CANVAS_TOKEN environment variable")
	}

	// fetch the object
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		log.Fatalf("Error creating HTTP request: %v", err)
	}
	req.Header.Add("Authorization", authHeader)

	// report the equivalent curl command
	//log.Printf(`curl -H "Authorization: Bearer $CANVAS_TOKEN" '%s'`, targetURL)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("GET error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		log.Fatalf("GET response %d: %s", resp.StatusCode, resp.Status)
	}
	if strings.Contains(resp.Header.Get("Link"), "rel=\"next\"") {
		log.Printf("while downloading %s", targetURL)
		log.Fatal("I only got a partial result! Bailing out so you do not proceed with missing data!")
	}

	if strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
		// decode it
		decoder := json.NewDecoder(resp.Body)
		if err = decoder.Decode(elt); err != nil {
			log.Printf("while downloading %s", targetURL)
			log.Fatalf("error decoding object: %v", err)
		}
	} else {
		if b, okay := elt.(*[]byte); okay {
			data, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Printf("while downloading %s", targetURL)
				log.Fatalf("error downloading response body: %v", err)
			}
			*b = data
		} else {
			log.Printf("while downloading %s", targetURL)
			log.Fatalf("response is not JSON and response object is not a byte slice")
		}
	}
}
