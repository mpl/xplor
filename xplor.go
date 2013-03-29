// 2010 - Mathieu Lonjaret

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"

	"bitbucket.org/fhs/goplumb/plumb"
	"code.google.com/p/goplan9/plan9"
	"code.google.com/p/goplan9/plan9/acme"
)

var (
	root       string
	w          *acme.Win
	PLAN9      = os.Getenv("PLAN9")
	showHidden bool
)

const (
	NBUF      = 512
	INDENT    = "	"
	dirflag   = "+ "
	nodirflag = "  "
)

type dir struct {
	charaddr string
	depth    int
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: xplor [path] \n")
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	flag.Usage = usage
	flag.Parse()

	args := flag.Args()

	switch len(args) {
	case 0:
		root, _ = os.Getwd()
	case 1:
		temp := path.Clean(args[0])
		if temp[0] != '/' {
			cwd, _ := os.Getwd()
			root = path.Join(cwd, temp)
		} else {
			root = temp
		}
	default:
		usage()
	}

	err := initWindow()
	if err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
		return
	}

	for word := range events() {
		if len(word) >= 6 && word[0:6] == "DotDot" {
			doDotDot()
			continue
		}
		if len(word) >= 6 && word[0:6] == "Hidden" {
			toggleHidden()
			continue
		}
		if len(word) >= 3 && word[0:3] == "Win" {
			if PLAN9 != "" {
				cmd := path.Join(PLAN9, "bin/win")
				doExec(word[3:len(word)], cmd)
			}
			continue
		}
		// yes, this does not cover all possible cases. I'll do better if anyone needs it.
		if len(word) >= 5 && word[0:5] == "Xplor" {
			cmd, err := exec.LookPath("xplor")
			if err != nil {
				fmt.Fprintf(os.Stderr, err.Error())
				continue
			}
			doExec(word[5:len(word)], cmd)
			continue
		}
		if word[0] == 'X' {
			onExec(word[1:len(word)])
			continue
		}
		onLook(word)
	}
}

func initWindow() error {
	var err error = nil
	w, err = acme.New()
	if err != nil {
		return err
	}

	title := "xplor-" + root
	w.Name(title)
	tag := "DotDot Win Xplor Hidden"
	w.Write("tag", []byte(tag))
	err = printDirContents(root, 0)
	return err
}

func printDirContents(dirpath string, depth int) (err error) {
	currentDir, err := os.OpenFile(dirpath, os.O_RDONLY, 0644)
	line := ""
	if err != nil {
		return err
	}
	names, err := currentDir.Readdirnames(-1)
	if err != nil {
		return err
	}
	currentDir.Close()

	sort.Strings(names)
	indents := ""
	for i := 0; i < depth; i++ {
		indents = indents + INDENT
	}
	fullpath := ""
	var fi os.FileInfo
	for _, v := range names {
		line = nodirflag + indents + v + "\n"
		isNotHidden := !strings.HasPrefix(v, ".")
		if isNotHidden || showHidden {
			fullpath = path.Join(dirpath, v)
			fi, err = os.Stat(fullpath)
			if err != nil {
				_, ok := err.(*os.PathError)
				if !ok {
					panic("Not a *os.PathError")
				}
				if !os.IsNotExist(err) {
					return err
				}
				// Skip (most likely) broken symlinks
				fmt.Fprintf(os.Stderr, "%v\n", err.Error())
				continue
			}
			if fi.IsDir() {
				line = dirflag + indents + v + "\n"
			}
			w.Write("data", []byte(line))
		}
	}

	if depth == 0 {
		//lame trick for now to dodge the out of range issue, until my address-foo gets better
		w.Write("body", []byte("\n"))
		w.Write("body", []byte("\n"))
		w.Write("body", []byte("\n"))
	}

	return err
}

func readLine(addr string) ([]byte, error) {
	var b []byte = make([]byte, NBUF)
	var err error = nil
	err = w.Addr("%s", addr)
	if err != nil {
		return b, err
	}
	n, err := w.Read("xdata", b)

	// remove dirflag, if any
	if n < 2 {
		return b[0 : n-1], err
	}
	return b[2 : n-1], err
}

func getDepth(line []byte) (depth int, trimedline string) {
	trimedline = strings.TrimLeft(string(line), INDENT)
	depth = (len(line) - len(trimedline)) / len(INDENT)
	return depth, trimedline
}

func isFolded(charaddr string) (bool, error) {
	var err error = nil
	var b []byte
	addr := "#" + charaddr + "+1-"
	b, err = readLine(addr)
	if err != nil {
		return true, err
	}
	depth, _ := getDepth(b)
	addr = "#" + charaddr + "+-"
	b, err = readLine(addr)
	if err != nil {
		return true, err
	}
	nextdepth, _ := getDepth(b)
	return (nextdepth <= depth), err
}

func getParents(charaddr string, depth int, prevline int) string {
	var addr string
	if depth == 0 {
		return ""
	}
	if prevline == 1 {
		addr = "#" + charaddr + "-+"
	} else {
		addr = "#" + charaddr + "-" + fmt.Sprint(prevline-1)
	}
	for {
		b, err := readLine(addr)
		if err != nil {
			fmt.Fprintf(os.Stderr, err.Error())
			os.Exit(1)
		}
		newdepth, line := getDepth(b)
		if newdepth < depth {
			fullpath := path.Join(getParents(charaddr, newdepth, prevline), line)
			return fullpath
		}
		prevline++
		addr = "#" + charaddr + "-" + fmt.Sprint(prevline-1)
	}
	return ""
}

// TODO(mpl): maybe break this one in a fold and unfold functions
func onLook(charaddr string) {
	// reconstruct full path and check if file or dir
	addr := "#" + charaddr + "+1-"
	b, err := readLine(addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
		return
	}
	depth, line := getDepth(b)
	fullpath := path.Join(root, getParents(charaddr, depth, 1), line)
	fi, err := os.Stat(fullpath)
	if err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
		return
	}

	if !fi.IsDir() {
		// not a dir -> send that file to the plumber
		port, err := plumb.Open("send", plan9.OWRITE)
		if err != nil {
			fmt.Fprintf(os.Stderr, err.Error())
			return
		}
		defer port.Close()
		port.Send(&plumb.Msg{
			Src:  "xplor",
			Dst:  "",
			WDir: "/",
			Kind: "text",
			Attr: map[string]string{},
			Data: []byte(fullpath),
		})
		return
	}

	folded, err := isFolded(charaddr)
	if err != nil {
		fmt.Fprint(os.Stderr, err.Error())
		return
	}
	if folded {
		// print dir contents
		addr = "#" + charaddr + "+2-1-#0"
		err = w.Addr("%s", addr)
		if err != nil {
			fmt.Fprintf(os.Stderr, err.Error()+addr)
			return
		}
		err = printDirContents(fullpath, depth+1)
		if err != nil {
			fmt.Fprint(os.Stderr, err.Error())
		}
	} else {
		// fold, ie delete lines below dir until we hit a dir of the same depth
		addr = "#" + charaddr + "+-"
		nextdepth := depth + 1
		nextline := 1
		for nextdepth > depth {
			err = w.Addr("%s", addr)
			if err != nil {
				fmt.Fprint(os.Stderr, err.Error())
				return
			}
			b, err = readLine(addr)
			if err != nil {
				fmt.Fprint(os.Stderr, err.Error())
				return
			}
			nextdepth, _ = getDepth(b)
			nextline++
			addr = "#" + charaddr + "+" + fmt.Sprint(nextline-1)
		}
		nextline--
		addr = "#" + charaddr + "+-#0,#" + charaddr + "+" + fmt.Sprint(nextline-2)
		err = w.Addr("%s", addr)
		if err != nil {
			fmt.Fprint(os.Stderr, err.Error())
			return
		}
		w.Write("data", []byte(""))
	}
}

func getFullPath(charaddr string) (fullpath string, err error) {
	// reconstruct full path and print it to Stdout
	addr := "#" + charaddr + "+1-"
	b, err := readLine(addr)
	if err != nil {
		return fullpath, err
	}
	depth, line := getDepth(b)
	fullpath = path.Join(root, getParents(charaddr, depth, 1), line)
	return fullpath, err
}

func doDotDot() {
	// blank the window
	err := w.Addr("0,$")
	if err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
		os.Exit(1)
	}
	w.Write("data", []byte(""))

	// restart from ..
	root = path.Clean(root + "/../")
	title := "xplor-" + root
	w.Name(title)
	err = printDirContents(root, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func doExec(loc string, cmd string) {
	var fullpath string
	if loc == "" {
		fullpath = root
	} else {
		var err error
		charaddr := strings.SplitAfterN(loc, ",#", 2)
		fullpath, err = getFullPath(charaddr[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, err.Error())
			return
		}
		fi, err := os.Stat(fullpath)
		if err != nil {
			fmt.Fprintf(os.Stderr, err.Error())
			return
		}
		if !fi.IsDir() {
			fullpath, _ = path.Split(fullpath)
		}
	}
	var args []string = make([]string, 1)
	args[0] = cmd
	fds := []*os.File{os.Stdin, os.Stdout, os.Stderr}
	_, err := os.StartProcess(args[0], args, &os.ProcAttr{Env: os.Environ(), Dir: fullpath, Files: fds})
	if err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
		return
	}
	return
}

// on a B2 click event we print the fullpath of the file to Stdout.
// This can come in handy for paths with spaces in it, because the
// plumber will fail to open them.  Printing it to Stdout allows us to do
// whatever we want with it when that happens.
// Also usefull with a dir path: once printed to stdout, a B3 click on
// the path to open it the "classic" acme way.
func onExec(charaddr string) {
	fullpath, err := getFullPath(charaddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
		return
	}
	fmt.Fprintf(os.Stderr, fullpath+"\n")
}

func toggleHidden() {
	showHidden = !showHidden
}

func events() <-chan string {
	c := make(chan string, 10)
	go func() {
		for e := range w.EventChan() {
			switch e.C2 {
			case 'x': // execute in tag
				switch string(e.Text) {
				case "Del":
					w.Ctl("delete")
				case "Hidden":
					c <- "Hidden"
				case "DotDot":
					c <- "DotDot"
				case "Win":
					tmp := ""
					if e.Flag != 0 {
						tmp = string(e.Loc)
					}
					c <- ("Win" + tmp)
				case "Xplor":
					tmp := ""
					if e.Flag != 0 {
						tmp = string(e.Loc)
					}
					c <- ("Xplor" + tmp)
				default:
					w.WriteEvent(e)
				}
			case 'X': // execute in body
				c <- ("X" + fmt.Sprint(e.OrigQ0))
			case 'l': // button 3 in tag
				// let the plumber deal with it
				w.WriteEvent(e)
			case 'L': // button 3 in body
				w.Ctl("clean")
				//ignore expansions
				if e.OrigQ0 != e.OrigQ1 {
					continue
				}
				c <- fmt.Sprint(e.OrigQ0)
			}
		}
		w.CloseFiles()
		close(c)
	}()
	return c
}
