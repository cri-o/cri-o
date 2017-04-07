#define _GNU_SOURCE
#include <ctype.h>
#include <errno.h>
#include <fcntl.h>
#include <limits.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/epoll.h>
#include <sys/prctl.h>
#include <sys/socket.h>
#include <sys/types.h>
#include <sys/un.h>
#include <sys/wait.h>
#include <syslog.h>
#include <unistd.h>

#include <glib.h>

#include "cmsg.h"

#define pexit(fmt, ...)                                                          \
	do {                                                                     \
		fprintf(stderr, "[conmon:e]: " fmt " %m\n", ##__VA_ARGS__);      \
		syslog(LOG_ERR, "conmon <error>: " fmt ": %m\n", ##__VA_ARGS__); \
		exit(EXIT_FAILURE);                                              \
	} while (0)

#define nexit(fmt, ...)                                                       \
	do {                                                                  \
		fprintf(stderr, "[conmon:e]: " fmt "\n", ##__VA_ARGS__);      \
		syslog(LOG_ERR, "conmon <error>: " fmt " \n", ##__VA_ARGS__); \
		exit(EXIT_FAILURE);                                           \
	} while (0)

#define nwarn(fmt, ...)                                                        \
	do {                                                                   \
		fprintf(stderr, "[conmon:w]: " fmt "\n", ##__VA_ARGS__);       \
	} while (0)

#define ninfo(fmt, ...)                                                        \
	do {                                                                   \
		fprintf(stderr, "[conmon:i]: " fmt "\n", ##__VA_ARGS__);       \
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
static char *log_path = NULL;
static GOptionEntry entries[] =
{
  { "terminal", 't', 0, G_OPTION_ARG_NONE, &terminal, "Terminal", NULL },
  { "cid", 'c', 0, G_OPTION_ARG_STRING, &cid, "Container ID", NULL },
  { "runtime", 'r', 0, G_OPTION_ARG_STRING, &runtime_path, "Runtime path", NULL },
  { "bundle", 'b', 0, G_OPTION_ARG_STRING, &bundle_path, "Bundle path", NULL },
  { "pidfile", 'p', 0, G_OPTION_ARG_STRING, &pid_file, "PID file", NULL },
  { "systemd-cgroup", 's', 0, G_OPTION_ARG_NONE, &systemd_cgroup, "Enable systemd cgroup manager", NULL },
  { "exec", 'e', 0, G_OPTION_ARG_NONE, &exec, "Exec a command in a running container", NULL },
  { "log-path", 'l', 0, G_OPTION_ARG_STRING, &log_path, "Log file path", NULL },
  { NULL }
};

int set_k8s_timestamp(char *buf, ssize_t buflen, const char *stream_type)
{
	time_t now = time(NULL);
	struct tm *tm;
	char off_sign = '+';
	int off;

	if ((tm = localtime(&now)) == NULL) {
		return -1;
	}
	off = (int) tm->tm_gmtoff;
	if (tm->tm_gmtoff < 0) {
		off_sign = '-';
		off = -off;
	}
	snprintf(buf, buflen, "%d-%02d-%02dT%02d:%02d:%02d%c%02d:%02d %s ",
		tm->tm_year + 1900, tm->tm_mon + 1, tm->tm_mday,
		tm->tm_hour, tm->tm_min, tm->tm_sec,
		off_sign, off / 3600, off % 3600, stream_type);

	return 0;
}


/*
 * splits buf into lines and inserts timestamps at the beginning of
 * the line written to logfd.
 */
int write_with_timestamps(int logfd, const char *buf, ssize_t buflen)
{
	#define TSBUFLEN 34
	char tsbuf[TSBUFLEN];
	static bool last_buf_ended_with_newline = TRUE;

	g_auto(GStrv) lines = g_strsplit(buf, "\n", -1);
	ssize_t num_lines = g_strv_length(lines);
	for (ssize_t i = 0; i < num_lines; i++)
	{
		const char *line = lines[i];

		ninfo("Processing line: %ld, %s", i, line);

		/* Skip last line if it is empty */
		if (i == (num_lines - 1) && buf[buflen-1] == '\n' && !strcmp("", line)) {
			ninfo("Skipping last line");
			break;
		}

		/*
		 * Only add timestamps for first line if last buffer's last
		 * line ended with newline. Add it for all other lines.
		 */
		if (i != 0 || (i == 0 && last_buf_ended_with_newline)) {
			ninfo("Adding timestamp");
			int rc = set_k8s_timestamp(tsbuf, TSBUFLEN, "stdout");
			if (rc < 0) {
				nwarn("failed to set timestamp");
			} else {
				/* Exclude the \0 while writing */
				if (write(logfd, tsbuf, TSBUFLEN - 1) != (TSBUFLEN - 1)) {
					nwarn("partial/failed write ts (logFd)");
				}
			}
		}

		/* Log output to logfd. */
		ssize_t len = strlen(line);
		if (write(logfd, line, len) != len) {
			nwarn("partial/failed write (logFd)");
			return -1;
		}
		/* Write the line ending */
		if ((i < num_lines - 1) || (i == (num_lines - 1) && buf[buflen - 1] == '\n')) {
			if (write(logfd, "\n", 1) != 1) {
				nwarn("failed to write line ending");
				return -1;
			}
		}
	}

	last_buf_ended_with_newline = buf[buflen-1] == '\n';

	return 0;
}

int main(int argc, char *argv[])
{
	int ret, runtime_status;
	char cwd[PATH_MAX];
	char default_pid_file[PATH_MAX];
	GError *err = NULL;
	_cleanup_free_ char *contents;
	int cpid = -1;
	int status;
	pid_t pid, create_pid;
	_cleanup_close_ int logfd = -1;
	_cleanup_close_ int mfd = -1;
	_cleanup_close_ int epfd = -1;
	_cleanup_close_ int csfd = -1;
	int runtime_mfd = -1;
	char csname[PATH_MAX] = "/tmp/conmon-term.XXXXXXXX";
	char buf[BUF_SIZE];
	int num_read;
	struct epoll_event ev;
	struct epoll_event evlist[MAX_EVENTS];
	int sync_pipe_fd = -1;
	char *sync_pipe, *endptr;
	int len;
	GError *error = NULL;
	GOptionContext *context;
	_cleanup_gstring_ GString *cmd = NULL;

	/* Command line parameters */
	context = g_option_context_new("- conmon utility");
	g_option_context_add_main_entries(context, entries, "conmon");
	if (!g_option_context_parse(context, &argc, &argv, &error)) {
	        g_print("option parsing failed: %s\n", error->message);
	        exit(1);
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

	if (log_path == NULL)
		nexit("Log file path not provided. Use --log-path");

	/* Environment variables */
	sync_pipe = getenv("_OCI_SYNCPIPE");
	if (sync_pipe) {
		errno = 0;
		sync_pipe_fd = strtol(sync_pipe, &endptr, 10);
		if (errno != 0 || *endptr != '\0')
			pexit("unable to parse _OCI_SYNCPIPE");
	}

	/* Open the log path file. */
	logfd = open(log_path, O_WRONLY | O_APPEND | O_CREAT);
	if (logfd < 0)
		pexit("Failed to open log file");

	/*
	 * Set self as subreaper so we can wait for container process
	 * and return its exit code.
	 */
	ret = prctl(PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0);
	if (ret != 0) {
		pexit("Failed to set as subreaper");
	}

	if (terminal) {
		struct sockaddr_un addr = {0};

		/*
		 * Generate a temporary name. Is this unsafe? Probably, but we can
		 * replace it with a rename(2) setup if necessary.
		 */

		int unusedfd = g_mkstemp(csname);
		if (unusedfd < 0)
			pexit("Failed to generate random path for console-socket");
		close(unusedfd);

		addr.sun_family = AF_UNIX;
		strncpy(addr.sun_path, csname, sizeof(addr.sun_path)-1);

		ninfo("addr{sun_family=AF_UNIX, sun_path=%s}", addr.sun_path);

		/* Bind to the console socket path. */
		csfd = socket(AF_UNIX, SOCK_STREAM|SOCK_CLOEXEC, 0);
		if (csfd < 0)
			pexit("Failed to create console-socket");
		/* XXX: This should be handled with a rename(2). */
		if (unlink(csname) < 0)
			pexit("Failed to unlink temporary ranom path");
		if (bind(csfd, (struct sockaddr *) &addr, sizeof(addr)) < 0)
			pexit("Failed to bind to console-socket");
		if (listen(csfd, 128) < 0)
			pexit("Failed to listen on console-socket");
	} else {
		int fds[2];

		/*
		 * Create a "fake" master fd so that we can use the same epoll code in
		 * both cases. The runtime_mfd will be closed after we dup over
		 * everything.
		 *
		 * We use pipes here because open(/dev/std{out,err}) will fail if we
		 * used anything else (and it wouldn't be a good idea to create a new
		 * pty pair in the host).
		 */
		if (pipe(fds) < 0)
			pexit("Failed to create runtime_mfd pipes");

		mfd = fds[0];
		runtime_mfd = fds[1];
	}

	cmd = g_string_new(runtime_path);

	/* Generate the cmdline. */
	if (exec && systemd_cgroup)
		g_string_append_printf(cmd, " --systemd-cgroup");

	if (exec)
		g_string_append_printf(cmd, " exec -d --pid-file %s", pid_file);
	else
		g_string_append_printf(cmd, " create --bundle %s --pid-file %s", bundle_path, pid_file);

	if (terminal)
		g_string_append_printf(cmd, " --console-socket %s", csname);

	/* Container name comes last. */
	g_string_append_printf(cmd, " %s", cid);

	/* Set the exec arguments. */
	if (exec) {
		/*
		 * FIXME: This code is broken if argv[1] contains spaces or other
		 *        similar characters that shells don't like. It's a bit silly
		 *        that we're doing things inside a shell at all -- this should
		 *        all be done in arrays.
		 */

		int i;
		for (i = 1; i < argc; i++)
			g_string_append_printf(cmd, " %s", argv[i]);
	}

	/*
	 * We have to fork here because the current runC API dups the stdio of the
	 * calling process over the container's fds. This is actually *very bad*
	 * but is currently being discussed for change in
	 * https://github.com/opencontainers/runtime-spec/pull/513. Hopefully this
	 * won't be the case for very long.
	 */

	/* Create our container. */
	create_pid = fork();
	if (create_pid < 0) {
		pexit("Failed to fork the create command");
	} else if (!create_pid) {
		char *argv[] = {"sh", "-c", cmd->str, NULL};

		/* We only need to touch the stdio if we have terminal=false. */
		/* FIXME: This results in us not outputting runc error messages to ocid's log. */
		if (runtime_mfd >= 0) {
			if (dup2(runtime_mfd, STDIN_FILENO) < 0)
				pexit("Failed to dup over stdin");
			if (dup2(runtime_mfd, STDOUT_FILENO) < 0)
				pexit("Failed to dup over stdout");
			if (dup2(runtime_mfd, STDERR_FILENO) < 0)
				pexit("Failed to dup over stderr");
		}

		/* Exec into the process. TODO: Don't use the shell. */
		execv("/bin/sh", argv);
		exit(127);
	}

	/* The runtime has that fd now. We don't need to touch it anymore. */
	close(runtime_mfd);

	/* Get the console fd. */
	/*
	 * FIXME: If runc fails to start a container, we won't bail because we're
	 *        busy waiting for requests. The solution probably involves
	 *        epoll(2) and a signalfd(2). This causes a lot of issues.
	 */
	if (terminal) {
		struct file_t console;
		int connfd = -1;

		ninfo("about to accept from csfd: %d", csfd);
		connfd = accept4(csfd, NULL, NULL, SOCK_CLOEXEC);
		if (connfd < 0)
			pexit("Failed to accept console-socket connection");

		/* Not accepting anything else. */
		close(csfd);
		unlink(csname);

		/* We exit if this fails. */
		ninfo("about to recvfd from connfd: %d", connfd);
		console = recvfd(connfd);

		ninfo("console = {.name = '%s'; .fd = %d}", console.name, console.fd);
		free(console.name);

		mfd = console.fd;

		/* Clean up everything */
		close(connfd);
	}

	ninfo("about to waitpid: %d", create_pid);

	/* Wait for our create child to exit with the return code. */
	if (waitpid(create_pid, &runtime_status, 0) < 0) {
		int old_errno = errno;
		kill(create_pid, SIGKILL);
		errno = old_errno;
		pexit("Failed to wait for `runtime %s`", exec ? "exec" : "create");
	}
	if (!WIFEXITED(runtime_status) || WEXITSTATUS(runtime_status) != 0)
		nexit("Failed to create container: exit status %d", WEXITSTATUS(runtime_status));

	/* Read the pid so we can wait for the process to exit */
	g_file_get_contents(pid_file, &contents, NULL, &err);
	if (err) {
		nwarn("Failed to read pidfile: %s", err->message);
		g_error_free(err);
		exit(1);
	}

	cpid = atoi(contents);
	ninfo("container PID: %d", cpid);

	/* Send the container pid back to parent */
	if (sync_pipe_fd > 0 && !exec) {
		len = snprintf(buf, BUF_SIZE, "{\"pid\": %d}\n", cpid);
		if (len < 0 || write(sync_pipe_fd, buf, len) != len) {
			pexit("unable to send container pid to parent");
		}
	}

	/* Create epoll_ctl so that we can handle read/write events. */
	/*
	 * TODO: Switch to libuv so that we can also implement exec as well as
	 *       attach and other important things. Using epoll directly is just
	 *       really nasty.
	 */
	epfd = epoll_create(5);
	if (epfd < 0)
		pexit("epoll_create");
	ev.events = EPOLLIN;
	/*
	ev.data.fd = STDIN_FILENO;
	if (epoll_ctl(epfd, EPOLL_CTL_ADD, STDIN_FILENO, &ev) < 0) {
		pexit("Failed to add stdin to epoll");
	}
	*/
	ev.data.fd = mfd;
	if (epoll_ctl(epfd, EPOLL_CTL_ADD, mfd, &ev) < 0) {
		pexit("Failed to add console master fd to epoll");
	}

	/*
	 * Log all of the container's output and pipe STDIN into it. Currently
	 * nothing using the STDIN setup (which makes its inclusion here a bit
	 * questionable but we need to rewrite this code soon anyway TODO).
	 */
	while (true) {
		int ready = epoll_wait(epfd, evlist, MAX_EVENTS, -1);
		int i = 0;
		for (i = 0; i < ready; i++) {
			if (evlist[i].events & EPOLLIN) {
				if (evlist[i].data.fd == STDIN_FILENO) {
					/*
					 * TODO: We need to replace STDIN_FILENO with something
					 *       more sophisticated so that attach actually works
					 *       properly.
					 */
					num_read = read(STDIN_FILENO, buf, BUF_SIZE);
					if (num_read <= 0)
						goto out;

					if (write(mfd, buf, num_read) != num_read) {
						nwarn("partial/failed write (masterFd)");
						goto out;
					}
				} else if (evlist[i].data.fd == mfd) {
					num_read = read(mfd, buf, BUF_SIZE);
					if (num_read <= 0)
						goto out;

					buf[num_read] = '\0';
					ninfo("read a chunk: (fd=%d) '%s'", mfd, buf);

					/* Insert CRI mandated timestamps in the buffer for each line */
					int rc = write_with_timestamps(logfd, buf, num_read);
					if (rc < 0) {
						goto out;
					}
				}
			} else if (evlist[i].events & (EPOLLHUP | EPOLLERR)) {
				printf("closing fd %d\n", evlist[i].data.fd);
				if (close(evlist[i].data.fd) < 0)
					pexit("close");
				goto out;
			}
		}
	}

out:
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
