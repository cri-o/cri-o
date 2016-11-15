#define _GNU_SOURCE
#include <ctype.h>
#include <errno.h>
#include <fcntl.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/epoll.h>
#include <sys/prctl.h>
#include <sys/wait.h>
#include <syslog.h>
#include <termios.h>
#include <unistd.h>

#include <glib.h>

#define pexit(fmt, ...)                                                          \
	do {                                                                     \
		fprintf(stderr, "conmon: " fmt " %m\n", ##__VA_ARGS__);          \
		syslog(LOG_ERR, "conmon <error>: " fmt ": %m\n", ##__VA_ARGS__); \
		exit(EXIT_FAILURE);                                              \
	} while (0)

#define nexit(fmt, ...)                                                       \
	do {                                                                  \
		fprintf(stderr, "conmon: " fmt "\n", ##__VA_ARGS__);          \
		syslog(LOG_ERR, "conmon <error>: " fmt " \n", ##__VA_ARGS__); \
		exit(EXIT_FAILURE);                                           \
	} while (0)

#define nwarn(fmt, ...)                                                        \
	do {                                                                   \
		fprintf(stderr, "conmon: " fmt "\n", ##__VA_ARGS__);           \
	} while (0)

#define _cleanup_(x) __attribute__((cleanup(x)))

static inline void freep(void *p)
{
	free(*(void **)p);
}

static inline void closep(int *fd)
{
	if (*fd >= 0)
		close(*fd);
	*fd = -1;
}

#define _cleanup_free_ _cleanup_(freep)
#define _cleanup_close_ _cleanup_(closep)

struct termios tty_orig;

static void tty_restore(void)
{
	if (tcsetattr(STDIN_FILENO, TCSANOW, &tty_orig) == -1)
		pexit("tcsetattr");
}

#define BUF_SIZE 256
#define CMD_SIZE 1024
#define MAX_EVENTS 10

static bool terminal = false;
static char *cid = NULL;
static char *runtime_path = NULL;
static GOptionEntry entries[] =
{
  { "terminal", 't', 0, G_OPTION_ARG_NONE, &terminal, "Terminal", NULL },
  { "cid", 'c', 0, G_OPTION_ARG_STRING, &cid, "Container ID", NULL },
  { "runtime", 'r', 0, G_OPTION_ARG_STRING, &runtime_path, "Runtime path", NULL },
  { NULL }
};

int main(int argc, char *argv[])
{
	int ret;
	char cmd[CMD_SIZE];
	GError *err = NULL;
	_cleanup_free_ char *contents;
	int cpid = -1;
	int status;
	pid_t pid;
	_cleanup_close_ int mfd = -1;
	_cleanup_close_ int epfd = -1;
	char slname[BUF_SIZE];
	char buf[BUF_SIZE];
	int num_read;
	struct termios t;
	struct epoll_event ev;
	struct epoll_event evlist[MAX_EVENTS];
	int child_pipe = -1;
	char *sync_pipe, *endptr;
	int len;
	GError *error = NULL;
	GOptionContext *context;

	/* Command line parameters */
	context = g_option_context_new ("- conmon utility");
	g_option_context_add_main_entries (context, entries, "conmon");
	if (!g_option_context_parse (context, &argc, &argv, &error)) {
	        g_print ("option parsing failed: %s\n", error->message);
	        exit (1);
	}

	if (cid == NULL)
		nexit("Container ID not provided. Use --cid");

	if (runtime_path == NULL)
		nexit("Runtime path not provided. Use --runtime");

	/* Environment variables */
	sync_pipe = getenv("_OCI_SYNCPIPE");
	if (sync_pipe) {
		errno = 0;
		child_pipe = strtol(sync_pipe, &endptr, 10);
		if (errno != 0 || *endptr != '\0')
			pexit("unable to parse _OCI_SYNCPIPE");
	}

	/*
	 * Set self as subreaper so we can wait for container process
	 * and return its exit code.
	 */
	ret = prctl(PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0);
	if (ret != 0) {
		pexit("Failed to set as subreaper");
	}

	if (terminal) {
		/* Open the master pty */
		mfd = open("/dev/ptmx", O_RDWR | O_NOCTTY);
		if (mfd < 0)
			pexit("Failed to open console master pty");

		/* Grant access to the slave pty */
		if (grantpt(mfd) == -1)
			pexit("Failed to grant access to slave pty");

		/* Unlock the slave pty */
		if (unlockpt(mfd) == -1) {	/* Unlock slave pty */
			pexit("Failed to unlock the slave pty");
		}

		/* Get the slave pty name */
		ret = ptsname_r(mfd, slname, BUF_SIZE);
		if (ret != 0) {
			pexit("Failed to get the slave pty name");
		}

	}

	/* Create the container */
	if (terminal) {
		snprintf(cmd, CMD_SIZE,
			 "%s create %s --pid-file pidfile --console %s",
			 runtime_path, cid, slname);
	} else {
		snprintf(cmd, CMD_SIZE, "%s create %s --pid-file pidfile",
			 runtime_path, cid);
	}
	ret = system(cmd);
	if (ret != 0) {
		nexit("Failed to create container");
	}

	/* Read the pid so we can wait for the process to exit */
	g_file_get_contents("pidfile", &contents, NULL, &err);
	if (err) {
		fprintf(stderr, "Failed to read pidfile: %s\n", err->message);
		g_error_free(err);
		exit(1);
	}

	cpid = atoi(contents);
	printf("container PID: %d\n", cpid);

	/* Send the container pid back to parent */
	if (child_pipe > 0) {
		len = snprintf(buf, BUF_SIZE, "{\"pid\": %d}\n", cpid);
		if (len < 0 || write(child_pipe, buf, len) != len) {
			pexit("unable to send container pid to parent");
		}
	}

	if (terminal) {
		/* Save exiting termios settings */
		if (tcgetattr(STDIN_FILENO, &tty_orig) == -1)
			pexit("tcegetattr");

		/* Settings for raw mode */
		t.c_lflag &=
		    ~(ISIG | ICANON | ECHO | ECHOE | ECHOK | ECHONL | IEXTEN);
		t.c_iflag &=
		    ~(BRKINT | ICRNL | IGNBRK | IGNCR | INLCR | INPCK | ISTRIP |
		      IXON | IXOFF | IGNPAR | PARMRK);
		t.c_oflag &= ~OPOST;
		t.c_cc[VMIN] = 1;
		t.c_cc[VTIME] = 0;

		/* Set terminal to raw mode */
		if (tcsetattr(STDIN_FILENO, TCSAFLUSH, &t) == -1)
			pexit("tcsetattr");

		/* Setup terminal restore on exit */
		if (atexit(tty_restore) != 0)
			pexit("atexit");

		epfd = epoll_create(5);
		if (epfd < 0)
			pexit("epoll_create");
		ev.events = EPOLLIN;
		ev.data.fd = STDIN_FILENO;
		if (epoll_ctl(epfd, EPOLL_CTL_ADD, STDIN_FILENO, &ev) < 0) {
			pexit("Failed to add stdin to epoll");
		}
		ev.data.fd = mfd;
		if (epoll_ctl(epfd, EPOLL_CTL_ADD, mfd, &ev) < 0) {
			pexit("Failed to add console master fd to epoll");
		}

		/* Copy data back and forth between STDIN and master fd */
		while (true) {
			int ready = epoll_wait(epfd, evlist, MAX_EVENTS, -1);
			int i = 0;
			for (i = 0; i < ready; i++) {
				if (evlist[i].events & EPOLLIN) {
					if (evlist[i].data.fd == STDIN_FILENO) {
						num_read =
						    read(STDIN_FILENO, buf,
							 BUF_SIZE);
						if (num_read <= 0)
							goto out;

						if (write(mfd, buf, num_read) !=
						    num_read) {
							nwarn
							    ("partial/failed write (masterFd)");
							goto out;
						}
					} else if (evlist[i].data.fd == mfd) {
						num_read =
						    read(mfd, buf, BUF_SIZE);
						if (num_read <= 0)
							goto out;

						if (write
						    (STDOUT_FILENO, buf,
						     num_read) != num_read) {
							nwarn
							    ("partial/failed write (STDOUT_FILENO)");
							goto out;
						}
					}
				} else if (evlist[i].events &
					   (EPOLLHUP | EPOLLERR)) {
					printf("closing fd %d\n",
					       evlist[i].data.fd);
					if (close(evlist[i].data.fd) < 0)
						pexit("close");
					goto out;
				}
			}
		}
 out:
		tty_restore();
	}

	/* Wait for the container process and record its exit code */
	while ((pid = waitpid(-1, &status, 0)) > 0) {
		printf("PID %d exited\n", pid);
		if (pid == cpid) {
			_cleanup_free_ char *status_str = NULL;
			ret = asprintf(&status_str, "%d", status);
			if (ret < 0) {
				pexit("Failed to allocate memory for status");
			}
			g_file_set_contents("exit", status_str,
					    strlen(status_str), &err);
			if (err) {
				fprintf(stderr,
					"Failed to write %s to exit file: %s\n",
					status_str, err->message);
				g_error_free(err);
				exit(1);
			}
			break;
		}
	}

	return EXIT_SUCCESS;
}
