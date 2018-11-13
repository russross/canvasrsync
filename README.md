Canvas assignment download tool
===============================

This is a simple command-line tool to sync all student submissions
in a Canvas course to a local directory.


Installation
------------

To install the tool itself, download the latest release for your
platform from the releases page:

*   https://github.com/russross/canvasrsync/releases

Put save it somewhere in your search path. For Linux or macOS, use
something like the following (assuming you saved it as `canvasrsync`
in your `Downloads` directory:

    sudo cp ~/Downloads/canvasrsync /usr/local/bin/
    sudo chmod 755 /usr/local/bin/canvasrsync

Type `canvasrsync` to see if it is recognized.


Setup
-----

To set it up, you need to create a token authorizing the app to
access your Canvas courses:

1.  In Canvas, select “Account” from the navigation bar on the left,
    then select “Settings” from the menu that pops out.

2.  Scroll down to the “Approvel Integrations:” section and click
    the “+ New Access Token” button. Fill in the popup (the
    “Purpose” field is just a note to yourself, so use something like
    “canvasrsync tool”, and the “Expires” field lets you put a limit on
    how long the token will be valid. I set it to the middle of the
    summer and re-generate a token every year).

3.  Copy the token from the popup. Once you close the window, you
    cannot get the token again (you have to generate a new one).

4.  In your command-line environment, create an environment variable
    called `CANVAS_TOKEN` whose value is the token you copied in
    step 3.

Once you have completed these steps, the tool will be authorized to
download anything you are authorized to download, which is generally
the student assignments in courses where you are an instructor.


Usage
-----

Here is the basic usage (you can get this by running `canvasrsync
-help`):


```
Usage: canvasrsync [flags] [filter terms]

If filter terms are supplied, a submission will only be downloaded
if every term is satisfied. A term is satisfied if it is a
case-insensitive substring match for any of the following:
  * Assignment name
  * Assignment description
  * Student login
  * Student name
  * Student email
If filter terms are used, the delete phase at the end is skipped.

Recognized flags:
  -course int
    	course ID (required)
  -despace
    	convert spaces in file names to underscores (default true)
  -dir string
    	directory to download into (default ".")
  -dry
    	dry run
```

To get a Canvas course ID, log in to Canvas, navigate to that
course, and then look at the number that appears in the URL. For
example, if the URL is:

    https://dixie.instructure.com/courses/12345

Then the course ID is 12345. In that case, you could download all
student assignments for that course using:

    canvasrsync -course 12345

This will:

*   Create a directory for the course
*   Within that directory, create a subdirectory for each assignment
*   Within each of those directories, create a subdirectory for each
    student
*   Within each student directory, download the files that student
    submitted

Directories that would otherwise be empty will be omitted. By
default, spaces are converted to underscores to make command-line
manipulation of files easier.

If you only want to download some submissions, use a filter:

    canvasrsync -course 12345 jane doe sudoku

This will only download a submission for a student if every filter
term (jane, doe, and sudoku) matched against the assignment name or
description, or the student name, login, or email. So you can filter
by student or by assignment or both.


File deletion
-------------

The tool will attempt to sync the course directory with the current
content on Canvas. This means that it will only download files that
are new or that have changed (unchanged files do not need to be
re-downloaded), but it also means that it will delete files in the
course directory that do not match what is currently in Canvas. This
normally happens when a student changes her submission or drops the
course, but it also means that files you create while grading will
be deleted on the next sync.

If you run the tool with search terms, deleting will only happen
within directories that were synced, i.e., within directories for
projects and students that matched the search terms. If you run it
with no search terms, it will delete any file in the directory tree
that it does not recognize.
