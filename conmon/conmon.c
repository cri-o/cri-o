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

static inline void gstring_free_cleanup(GString **string)
{
	if (*string)
		g_string_free(*string, TRUE);
}

#define _cleanup_free_ _cleanup_(freep)
#define _cleanup_close_ _cleanup_(closep)
#define _cleanup_gstring_ _cleanup_(gstring_free_cleanup)

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
static char *bundle_path = NULL;
static char *pid_file = NULL;
static bool systemd_cgroup = false;
static bool exec = false;
static GOptionEntry entries[] =
{
  { "terminal", 't', 0, G_OPTION_ARG_NONE, &terminal, "Terminal", NULL },
  { "cid", 'c', 0, G_OPTION_ARG_STRING, &cid, "Container ID", NULL },
  { "runtime", 'r', 0, G_OPTION_ARG_STRING, &runtime_path, "Runtime path", NULL },
  { "bundle", 'b', 0, G_OPTION_ARG_STRING, &bundle_path, "Bundle path", NULL },
  { "pidfile", 'p', 0, G_OPTION_ARG_STRING, &pid_file, "PID file", NULL },
  { "systemd-cgroup", 's', 0, G_OPTION_ARG_NONE, &systemd_cgroup, "Enable systemd cgroup manager", NULL },
  { "exec", 'e', 0, G_OPTION_ARG_NONE, &exec, "Exec a command in a running container", NULL },
  { NULL }
};

int main(int argc, char *argv[])
{
	int ret, runtime_status;
	char cwd[PATH_MAX];
	char default_pid_file[PATH_MAX];
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
	int sync_pipe_fd = -1;
	char *sync_pipe, *endptr;
	int len;
	GError *error = NULL;
	GOptionContext *context;
	_cleanup_gstring_ GString *cmd = NULL;

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

	if (bundle_path == NULL && !exec) {
		if (getcwd(cwd, sizeof(cwd)) == NULL) {
			nexit("Failed to get working directory");
		}

		bundle_path = cwd;
	}

	if (pid_file == NULL) {
		if (snprintf(default_pid_file, sizeof(default_pid_file),
			     "%s/pidfile-%s", cwd, cid) < 0) {
			nexit("Failed to generate the pidfile path");
		}

		pid_file = default_pid_file;
	}

	/* Environment variables */
	sync_pipe = getenv("_OCI_SYNCPIPE");
	if (sync_pipe) {
		errno = 0;
		sync_pipe_fd = strtol(sync_pipe, &endptr, 10);
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

	cmd = g_string_new(runtime_path);
	if (!exec) {
		/* Create the container */
		if (systemd_cgroup) {
			g_string_append_printf(cmd, " --systemd-cgroup");
		}
		g_string_append_printf(cmd, " create %s --bundle %s --pid-file %s",
				       cid, bundle_path, pid_file);
		if (terminal) {
			g_string_append_printf(cmd, " --console %s", slname);
		}
	} else {
		int i;

		/* Exec the command */
		if (terminal) {
			g_string_append_printf(cmd, " exec -d --pid-file %s --console %s %s",
					       pid_file, slname, cid);
		} else {
			g_string_append_printf(cmd, " exec -d --pid-file %s %s",
					       pid_file, cid);
		}

		for (i = 1; i < argc; i++) {
			g_string_append_printf(cmd, " %s", argv[i]);
		}
	}

	runtime_status = system(cmd->str);
	if (runtime_status != 0) {
		nexit("Failed to create container");
	}

	/* Read the pid so we can wait for the process to exit */
	g_file_get_contents(pid_file, &contents, NULL, &err);
	if (err) {
		fprintf(stderr, "Failed to read pidfile: %s\n", err->message);
		g_error_free(err);
		exit(1);
	}

	cpid = atoi(contents);
	printf("container PID: %d\n", cpid);

	/* Send the container pid back to parent */
	if (sync_pipe_fd > 0  && !exec) {
		len = snprintf(buf, BUF_SIZE, "{\"pid\": %d}\n", cpid);
		if (len < 0 || write(sync_pipe_fd, buf, len) != len) {
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
		int exit_status = WEXITSTATUS(status);

		printf("PID %d exited with status %d\n", pid, exit_status);
		if (pid == cpid) {
			if (!exec) {
				_cleanup_free_ char *status_str = NULL;
				ret = asprintf(&status_str, "%d", exit_status);
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
			} else {
				/* Send the command exec exit code back to the parent */
				if (sync_pipe_fd > 0) {
					len = snprintf(buf, BUF_SIZE, "{\"exit_code\": %d}\n", exit_status);
					if (len < 0 || write(sync_pipe_fd, buf, len) != len) {
						pexit("unable to send exit status");
						exit(1);
					}
				}
			}
			break;
		}
	}

	if (exec && pid < 0 && errno == ECHILD && sync_pipe_fd > 0) {
		/*
		 * waitpid failed and set errno to ECHILD:
		 * The runtime exec call did not create any child
		 * process and we can send the system() exit code
		 * to the parent.
		 */
		len = snprintf(buf, BUF_SIZE, "{\"exit_code\": %d}\n", WEXITSTATUS(runtime_status));
		if (len < 0 || write(sync_pipe_fd, buf, len) != len) {
			pexit("unable to send exit status");
			exit(1);
		}
	}

	return EXIT_SUCCESS;
}
