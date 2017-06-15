#define _GNU_SOURCE
#include <ctype.h>
#include <errno.h>
#include <fcntl.h>
#include <limits.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/prctl.h>
#include <sys/socket.h>
#include <sys/types.h>
#include <sys/un.h>
#include <sys/stat.h>
#include <sys/wait.h>
#include <sys/eventfd.h>
#include <sys/stat.h>
#include <sys/uio.h>
#include <sys/ioctl.h>
#include <syslog.h>
#include <unistd.h>

#include <glib.h>
#include <glib-unix.h>

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
		syslog(LOG_INFO, "conmon <nwarn>: " fmt " \n", ##__VA_ARGS__); \
	} while (0)

#define ninfo(fmt, ...)                                                        \
	do {                                                                   \
		fprintf(stderr, "[conmon:i]: " fmt "\n", ##__VA_ARGS__);       \
		syslog(LOG_INFO, "conmon <ninfo>: " fmt " \n", ##__VA_ARGS__); \
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

static inline void fclosep(FILE **fp) {
	if (*fp)
		fclose(*fp);
	*fp = NULL;
}

static inline void gstring_free_cleanup(GString **string)
{
	if (*string)
		g_string_free(*string, TRUE);
}

static inline void strv_cleanup(char ***strv)
{
	if (strv)
		g_strfreev (*strv);
}

#define _cleanup_free_ _cleanup_(freep)
#define _cleanup_close_ _cleanup_(closep)
#define _cleanup_fclose_ _cleanup_(fclosep)
#define _cleanup_gstring_ _cleanup_(gstring_free_cleanup)
#define _cleanup_strv_ _cleanup_(strv_cleanup)

#define BUF_SIZE 8192
#define CMD_SIZE 1024
#define MAX_EVENTS 10

static bool terminal = false;
static bool opt_stdin = false;
static char *cid = NULL;
static char *cuuid = NULL;
static char *runtime_path = NULL;
static char *bundle_path = NULL;
static char *pid_file = NULL;
static bool systemd_cgroup = false;
static char *exec_process_spec = NULL;
static bool exec = false;
static char *log_path = NULL;
static GOptionEntry entries[] =
{
  { "terminal", 't', 0, G_OPTION_ARG_NONE, &terminal, "Terminal", NULL },
  { "stdin", 'i', 0, G_OPTION_ARG_NONE, &opt_stdin, "Stdin", NULL },
  { "cid", 'c', 0, G_OPTION_ARG_STRING, &cid, "Container ID", NULL },
  { "cuuid", 'u', 0, G_OPTION_ARG_STRING, &cuuid, "Container UUID", NULL },
  { "runtime", 'r', 0, G_OPTION_ARG_STRING, &runtime_path, "Runtime path", NULL },
  { "bundle", 'b', 0, G_OPTION_ARG_STRING, &bundle_path, "Bundle path", NULL },
  { "pidfile", 'p', 0, G_OPTION_ARG_STRING, &pid_file, "PID file", NULL },
  { "systemd-cgroup", 's', 0, G_OPTION_ARG_NONE, &systemd_cgroup, "Enable systemd cgroup manager", NULL },
  { "exec", 'e', 0, G_OPTION_ARG_NONE, &exec, "Exec a command in a running container", NULL },
  { "exec-process-spec", 0, 0, G_OPTION_ARG_STRING, &exec_process_spec, "Path to the process spec for exec", NULL },
  { "log-path", 'l', 0, G_OPTION_ARG_STRING, &log_path, "Log file path", NULL },
  { NULL }
};

/* strlen("1997-03-25T13:20:42.999999999+01:00 stdout ") + 1 */
#define TSBUFLEN 44

#define CGROUP_ROOT "/sys/fs/cgroup"

static ssize_t write_all(int fd, const void *buf, size_t count)
{
	size_t remaining = count;
	const char *p = buf;
	ssize_t res;

	while (remaining > 0) {
		do {
			res = write(fd, p, remaining);
		} while (res == -1 && errno == EINTR);

		if (res <= 0)
			return -1;

		remaining -= res;
		p += res;
	}

	return count;
}

#define WRITEV_BUFFER_N_IOV 128

typedef struct {
	int iovcnt;
	struct iovec iov[WRITEV_BUFFER_N_IOV];
} writev_buffer_t;

static ssize_t writev_buffer_flush (int fd, writev_buffer_t *buf)
{
	size_t count = 0;
	ssize_t res;
	struct iovec *iov;
	int iovcnt;

	iovcnt = buf->iovcnt;
	iov = buf->iov;

	while (iovcnt > 0) {
		do {
			res = writev(fd, iov, iovcnt);
		} while (res == -1 && errno == EINTR);

		if (res <= 0)
			return -1;

		count += res;

		while (res > 0) {
			size_t from_this = MIN((size_t)res, iov->iov_len);
			iov->iov_len -= from_this;
			res -= from_this;

			if (iov->iov_len == 0) {
				iov++;
				iovcnt--;
			}
		}
	}

	buf->iovcnt = 0;

	return count;
}

ssize_t writev_buffer_append_segment(int fd, writev_buffer_t *buf, const void *data, ssize_t len)
{
	if (data == NULL)
		return 1;

	if (len < 0)
		len = strlen ((char *)data);

	if (buf->iovcnt == WRITEV_BUFFER_N_IOV &&
	    writev_buffer_flush (fd, buf) < 0)
		return -1;

	if (len > 0) {
		buf->iov[buf->iovcnt].iov_base = (void *)data;
		buf->iov[buf->iovcnt].iov_len = (size_t)len;
		buf->iovcnt++;
	}

	return 1;
}

int set_k8s_timestamp(char *buf, ssize_t buflen, const char *pipename)
{
	struct tm *tm;
	struct timespec ts;
	char off_sign = '+';
	int off, len, err = -1;

	if (clock_gettime(CLOCK_REALTIME, &ts) < 0) {
		/* If CLOCK_REALTIME is not supported, we set nano seconds to 0 */
		if (errno == EINVAL) {
			ts.tv_nsec = 0;
		} else {
			return err;
		}
	}

	if ((tm = localtime(&ts.tv_sec)) == NULL)
		return err;


	off = (int) tm->tm_gmtoff;
	if (tm->tm_gmtoff < 0) {
		off_sign = '-';
		off = -off;
	}

	len = snprintf(buf, buflen, "%d-%02d-%02dT%02d:%02d:%02d.%09ld%c%02d:%02d %s ",
		       tm->tm_year + 1900, tm->tm_mon + 1, tm->tm_mday,
		       tm->tm_hour, tm->tm_min, tm->tm_sec, ts.tv_nsec,
		       off_sign, off / 3600, off % 3600, pipename);

	if (len < buflen)
		err = 0;
	return err;
}

/* stdpipe_t represents one of the std pipes (or NONE).
 * Sync with const in container_attach.go */
typedef enum {
	NO_PIPE,
	STDIN_PIPE, /* unused */
	STDOUT_PIPE,
	STDERR_PIPE,
} stdpipe_t;

const char *stdpipe_name(stdpipe_t pipe)
{
	switch (pipe) {
	case STDIN_PIPE:
		return "stdin";
	case STDOUT_PIPE:
		return "stdout";
	case STDERR_PIPE:
		return "stderr";
	default:
		return "NONE";
	}
}

/*
 * The CRI requires us to write logs with a (timestamp, stream, line) format
 * for every newline-separated line. write_k8s_log writes said format for every
 * line in buf, and will partially write the final line of the log if buf is
 * not terminated by a newline.
 */
int write_k8s_log(int fd, stdpipe_t pipe, const char *buf, ssize_t buflen)
{
	char tsbuf[TSBUFLEN];
	static stdpipe_t trailing_line = NO_PIPE;
	writev_buffer_t bufv = {0};

	/*
	 * Use the same timestamp for every line of the log in this buffer.
	 * There is no practical difference in the output since write(2) is
	 * fast.
	 */
	if (set_k8s_timestamp(tsbuf, sizeof tsbuf, stdpipe_name(pipe)))
		/* TODO: We should handle failures much more cleanly than this. */
		return -1;

	while (buflen > 0) {
		const char *line_end = NULL;
		ptrdiff_t line_len = 0;

		/* Find the end of the line, or alternatively the end of the buffer. */
		line_end = memchr(buf, '\n', buflen);
		if (line_end == NULL)
			line_end = &buf[buflen-1];
		line_len = line_end - buf + 1;

		/*
		 * Write the (timestamp, stream) tuple if there isn't any trailing
		 * output from the previous line (or if there is trailing output but
		 * the current buffer being printed is from a different pipe).
		 */
		if (trailing_line != pipe) {
			/*
			 * If there was a trailing line from a different pipe, prepend a
			 * newline to split it properly. This technically breaks the flow
			 * of the previous line (adding a newline in the log where there
			 * wasn't one output) but without modifying the file in a
			 * non-append-only way there's not much we can do.
			 */
			if ((trailing_line != NO_PIPE &&
			     writev_buffer_append_segment(fd, &bufv, "\n", -1) < 0) ||
			    writev_buffer_append_segment(fd, &bufv, tsbuf, -1) < 0) {
				nwarn("failed to write (timestamp, stream) to log");
				goto next;
			}
		}

		/* Output the actual contents. */
		if (writev_buffer_append_segment(fd, &bufv, buf, line_len) < 0) {
			nwarn("failed to write buffer to log");
			goto next;
		}

		/* If we did not output a full line, then we are a trailing_line. */
		trailing_line = (*line_end == '\n') ? NO_PIPE : pipe;

next:
		/* Update the head of the buffer remaining to output. */
		buf += line_len;
		buflen -= line_len;
	}

	if (writev_buffer_flush (fd, &bufv) < 0) {
		nwarn("failed to flush buffer to log");
	}

	return 0;
}

/*
 * Returns the path for specified controller name for a pid.
 * Returns NULL on error.
 */
static char *process_cgroup_subsystem_path(int pid, const char *subsystem) {
	_cleanup_free_ char *cgroups_file_path = NULL;
	int rc;
	rc = asprintf(&cgroups_file_path, "/proc/%d/cgroup", pid);
	if (rc < 0) {
		nwarn("Failed to allocate memory for cgroups file path");
		return NULL;
	}

	_cleanup_fclose_ FILE *fp = NULL;
	fp = fopen(cgroups_file_path, "re");
	if (fp == NULL) {
		nwarn("Failed to open cgroups file: %s", cgroups_file_path);
		return NULL;
	}

	_cleanup_free_ char *line = NULL;
	ssize_t read;
	size_t len = 0;
	char *ptr, *path;
	char *subsystem_path = NULL;
	int i;
	while ((read = getline(&line, &len, fp)) != -1) {
		_cleanup_strv_ char **subsystems = NULL;
		ptr = strchr(line, ':');
		if (ptr == NULL) {
			nwarn("Error parsing cgroup, ':' not found: %s", line);
			return NULL;
		}
		ptr++;
		path = strchr(ptr, ':');
		if (path == NULL) {
			nwarn("Error parsing cgroup, second ':' not found: %s", line);
			return NULL;
		}
		*path = 0;
		path++;
		subsystems = g_strsplit (ptr, ",", -1);
		for (i = 0; subsystems[i] != NULL; i++) {
			if (strcmp (subsystems[i], subsystem) == 0) {
				char *subpath = strchr(subsystems[i], '=');
				if (subpath == NULL) {
					subpath = ptr;
				} else {
					*subpath = 0;
				}

				rc = asprintf(&subsystem_path, "%s/%s%s", CGROUP_ROOT, subpath, path);
				if (rc < 0) {
					nwarn("Failed to allocate memory for subsystemd path");
					return NULL;
				}

				subsystem_path[strlen(subsystem_path) - 1] = '\0';
				return subsystem_path;
			}
		}
	}

	return NULL;
}

static char *escape_json_string(const char *str)
{
	GString *escaped;
	const char *p;

	p = str;
	escaped = g_string_sized_new(strlen(str));

	while (*p != 0) {
		char c = *p++;
		if (c == '\\' || c == '"') {
			g_string_append_c(escaped, '\\');
			g_string_append_c(escaped, c);
		} else if (c == '\n') {
			g_string_append_printf (escaped, "\\n");
		} else if (c == '\t') {
			g_string_append_printf (escaped, "\\t");
		} else if ((c > 0 && c < 0x1f) || c == 0x7f) {
			g_string_append_printf (escaped, "\\u00%02x", (guint)c);
		} else {
			g_string_append_c (escaped, c);
		}
	}

	return g_string_free (escaped, FALSE);
}


/* Global state */

static int runtime_status = -1;

static int masterfd_stdin = -1;
static int masterfd_stdout = -1;
static int masterfd_stderr = -1;
static int num_stdio_fds = 0;

/* Used for attach */
static int conn_sock = -1;
static int conn_sock_readable;
static int conn_sock_writable;

static int logfd = -1;
static int oom_efd = -1;
static int afd = -1;
static int cfd = -1;
/* Used for OOM notification API */
static int ofd = -1;

static GMainLoop *main_loop = NULL;

static void conn_sock_shutdown(int how)
{
	if (conn_sock == -1)
		return;
	shutdown(conn_sock, how);
	if (how & SHUT_RD)
		conn_sock_readable = false;
	if (how & SHUT_WR)
		conn_sock_writable = false;
	if (!conn_sock_writable && !conn_sock_readable) {
		close(conn_sock);
		conn_sock = -1;
	}
}

static gboolean stdio_cb(int fd, GIOCondition condition, gpointer user_data)
{
	#define STDIO_BUF_SIZE 8192 /* Sync with redirectResponseToOutputStreams() */
	/* We use one extra byte at the start, which we don't read into, instead
	   we use that for marking the pipe when we write to the attached socket */
	char real_buf[STDIO_BUF_SIZE + 1];
        char *buf = real_buf + 1;
	stdpipe_t pipe = GPOINTER_TO_INT(user_data);
	ssize_t num_read = 0;

	if ((condition & G_IO_IN) != 0) {
		num_read = read(fd, buf, BUF_SIZE);
		if (num_read < 0) {
			nwarn("stdio_input read failed %s", strerror(errno));
			return G_SOURCE_CONTINUE;
		}

		if (num_read > 0) {
			if (write_k8s_log(logfd, pipe, buf, num_read) < 0) {
				nwarn("write_k8s_log failed");
				return G_SOURCE_CONTINUE;
			}

                        real_buf[0] = pipe;
			if (conn_sock_writable && write_all(conn_sock, real_buf, num_read+1) < 0) {
				nwarn("Failed to write to socket");
				conn_sock_shutdown(SHUT_WR);
			}

			return G_SOURCE_CONTINUE;
		}
	}

	/* End of input */
	if (pipe == STDOUT_PIPE)
		masterfd_stdout = -1;
	if (pipe == STDERR_PIPE)
		masterfd_stderr = -1;
	num_stdio_fds--;
	if (num_stdio_fds == 0) {
		ninfo ("No more stdio, killing main loop");
		g_main_loop_quit (main_loop);
	}

	close (fd);
	return G_SOURCE_REMOVE;
}

static gboolean oom_cb(int fd, GIOCondition condition, G_GNUC_UNUSED gpointer user_data)
{
	uint64_t oom_event;
	ssize_t num_read = 0;

	if ((condition & G_IO_IN) != 0) {
		num_read = read(fd, &oom_event, sizeof(uint64_t));
		if (num_read < 0) {
			nwarn("Failed to read oom event from eventfd");
			return G_SOURCE_CONTINUE;
		}

		if (num_read > 0) {
			if (num_read != sizeof(uint64_t))
				nwarn("Failed to read full oom event from eventfd");
			ninfo("OOM received");
			if (open("oom", O_CREAT, 0666) < 0) {
				nwarn("Failed to write oom file");
			}
			return G_SOURCE_CONTINUE;
		}
	}

	/* End of input */
	close (fd);
	oom_efd = -1;
	return G_SOURCE_REMOVE;
}

static gboolean conn_sock_cb(int fd, GIOCondition condition, G_GNUC_UNUSED gpointer user_data)
{
        #define CONN_SOCK_BUF_SIZE 32*1024 /* Match the write size in CopyDetachable */
	char buf[CONN_SOCK_BUF_SIZE];
	ssize_t num_read = 0;

	if ((condition & G_IO_IN) != 0) {
		num_read = read(fd, buf, CONN_SOCK_BUF_SIZE);
		if (num_read < 0)
			return G_SOURCE_CONTINUE;

		if (num_read > 0 && masterfd_stdin >= 0) {
			if (write_all(masterfd_stdin, buf, num_read) < 0) {
				nwarn("Failed to write to container stdin");
			}
			return G_SOURCE_CONTINUE;
		}
	}

	/* End of input */
	conn_sock_shutdown(SHUT_RD);
	if (masterfd_stdin >= 0 && opt_stdin) {
		close(masterfd_stdin);
		masterfd_stdin = -1;
	}
	return G_SOURCE_REMOVE;
}

static gboolean attach_cb(int fd, G_GNUC_UNUSED GIOCondition condition, G_GNUC_UNUSED gpointer user_data)
{
	conn_sock = accept(fd, NULL, NULL);
	if (conn_sock == -1) {
		if (errno != EWOULDBLOCK)
			nwarn("Failed to accept client connection on attach socket");
	} else {
		conn_sock_readable = true;
		conn_sock_writable = true;
		g_unix_fd_add (conn_sock, G_IO_IN|G_IO_HUP|G_IO_ERR, conn_sock_cb, GINT_TO_POINTER(STDOUT_PIPE));
		ninfo("Accepted connection %d", conn_sock);
	}

	return G_SOURCE_CONTINUE;
}

static gboolean ctrl_cb(int fd, G_GNUC_UNUSED GIOCondition condition, G_GNUC_UNUSED gpointer user_data)
{
	#define CTLBUFSZ 200
	static char ctlbuf[CTLBUFSZ];
	static int readsz = CTLBUFSZ - 1;
	static char *readptr = ctlbuf;
	ssize_t num_read = 0;
	int ctl_msg_type = -1;
	int height = -1;
	int width = -1;
	struct winsize ws;
	int ret;

	num_read = read(fd, readptr, readsz);
	if (num_read <= 0) {
		nwarn("Failed to read from control fd");
		return G_SOURCE_CONTINUE;
	}

	readptr[num_read] = '\0';
	ninfo("Got ctl message: %s\n", ctlbuf);

	char *beg = ctlbuf;
	char *newline = strchrnul(beg, '\n');
	/* Process each message which ends with a line */
	while (*newline != '\0') {
		ret = sscanf(ctlbuf, "%d %d %d\n", &ctl_msg_type, &height, &width);
		if (ret != 3) {
			nwarn("Failed to sscanf message");
			return G_SOURCE_CONTINUE;
		}
		ninfo("Message type: %d, Height: %d, Width: %d", ctl_msg_type, height, width);
		ret = ioctl(masterfd_stdout, TIOCGWINSZ, &ws);
		ninfo("Existing size: %d %d", ws.ws_row, ws.ws_col);
		ws.ws_row = height;
		ws.ws_col = width;
		ret = ioctl(masterfd_stdout, TIOCSWINSZ, &ws);
		if (ret == -1) {
			nwarn("Failed to set process pty terminal size");
		}
		beg = newline + 1;
		newline = strchrnul(beg, '\n');
	}
	if (num_read == (CTLBUFSZ - 1) && beg == ctlbuf) {
		/*
		 * We did not find a newline in the entire buffer.
		 * This shouldn't happen as our buffer is larger than
		 * the message that we expect to receive.
		 */
		nwarn("Could not find newline in entire buffer\n");
	} else if (*beg == '\0') {
		/* We exhausted all messages that were complete */
		readptr = ctlbuf;
		readsz = CTLBUFSZ - 1;
	} else {
		/*
		 * We copy remaining data to beginning of buffer
		 * and advance readptr after that.
		 */
		int cp_rem = 0;
		do {
			ctlbuf[cp_rem++] = *beg++;
		} while (*beg != '\0');
		readptr = ctlbuf + cp_rem;
		readsz = CTLBUFSZ - 1 - cp_rem;
	}

	return G_SOURCE_CONTINUE;
}

static gboolean terminal_accept_cb(int fd, G_GNUC_UNUSED GIOCondition condition, G_GNUC_UNUSED gpointer user_data)
{
	const char *csname = user_data;
	struct file_t console;
	int connfd = -1;

	ninfo("about to accept from csfd: %d", fd);
	connfd = accept4(fd, NULL, NULL, SOCK_CLOEXEC);
	if (connfd < 0)
		pexit("Failed to accept console-socket connection");

	/* Not accepting anything else. */
	close(fd);
	unlink(csname);

	/* We exit if this fails. */
	ninfo("about to recvfd from connfd: %d", connfd);
	console = recvfd(connfd);

	ninfo("console = {.name = '%s'; .fd = %d}", console.name, console.fd);
	free(console.name);

	/* We only have a single fd for both pipes, so we just treat it as
	 * stdout. stderr is ignored. */
	masterfd_stdin = console.fd;
	masterfd_stdout = console.fd;
	masterfd_stderr = -1;

	/* Clean up everything */
	close(connfd);

	return G_SOURCE_CONTINUE;
}

static void
runtime_exit_cb (G_GNUC_UNUSED GPid pid, int status, G_GNUC_UNUSED gpointer user_data)
{
	runtime_status = status;
	g_main_loop_quit (main_loop);
}

int main(int argc, char *argv[])
{
	int ret;
	char cwd[PATH_MAX];
	char default_pid_file[PATH_MAX];
	char attach_sock_path[PATH_MAX];
	char ctl_fifo_path[PATH_MAX];
	GError *err = NULL;
	_cleanup_free_ char *contents;
	int cpid = -1;
	int status;
	pid_t pid, create_pid;
	_cleanup_close_ int epfd = -1;
	_cleanup_close_ int csfd = -1;
	/* Used for !terminal cases. */
	int slavefd_stdin = -1;
	int slavefd_stdout = -1;
	int slavefd_stderr = -1;
	char csname[PATH_MAX] = "/tmp/conmon-term.XXXXXXXX";
	char buf[BUF_SIZE];
	int num_read;
	int sync_pipe_fd = -1;
	char *sync_pipe, *endptr;
	int len;
	GError *error = NULL;
	GOptionContext *context;
        GPtrArray *runtime_argv = NULL;

	_cleanup_free_ char *memory_cgroup_path = NULL;
	int wb;

	main_loop = g_main_loop_new (NULL, FALSE);

	/* Command line parameters */
	context = g_option_context_new("- conmon utility");
	g_option_context_add_main_entries(context, entries, "conmon");
	if (!g_option_context_parse(context, &argc, &argv, &error)) {
	        g_print("option parsing failed: %s\n", error->message);
	        exit(1);
	}

	if (cid == NULL)
		nexit("Container ID not provided. Use --cid");

	if (!exec && cuuid == NULL)
		nexit("Container UUID not provided. Use --cuuid");

	if (runtime_path == NULL)
		nexit("Runtime path not provided. Use --runtime");

	if (bundle_path == NULL && !exec) {
		if (getcwd(cwd, sizeof(cwd)) == NULL) {
			nexit("Failed to get working directory");
		}
		bundle_path = cwd;
	}

	if (exec && exec_process_spec == NULL) {
		nexit("Exec process spec path not provided. Use --exec-process-spec");
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
		if (fcntl(sync_pipe_fd, F_SETFD, FD_CLOEXEC) == -1)
			pexit("unable to make _OCI_SYNCPIPE CLOEXEC");
	}

	/* Open the log path file. */
	logfd = open(log_path, O_WRONLY | O_APPEND | O_CREAT | O_CLOEXEC, 0600);
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
		if (fchmod(csfd, 0700))
			pexit("Failed to change console-socket permissions");
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
		 * both cases. The slavefd_*s will be closed after we dup over
		 * everything.
		 *
		 * We use pipes here because open(/dev/std{out,err}) will fail if we
		 * used anything else (and it wouldn't be a good idea to create a new
		 * pty pair in the host).
		 */

		if (opt_stdin) {
			if (pipe2(fds, O_CLOEXEC) < 0)
				pexit("Failed to create !terminal stdin pipe");

			masterfd_stdin = fds[1];
			slavefd_stdin = fds[0];
		}

		if (pipe2(fds, O_CLOEXEC) < 0)
			pexit("Failed to create !terminal stdout pipe");

		masterfd_stdout = fds[0];
		slavefd_stdout = fds[1];

		if (pipe2(fds, O_CLOEXEC) < 0)
			pexit("Failed to create !terminal stderr pipe");

		masterfd_stderr = fds[0];
		slavefd_stderr = fds[1];
	}

	runtime_argv = g_ptr_array_new();
	g_ptr_array_add(runtime_argv, runtime_path);

	/* Generate the cmdline. */
	if (!exec && systemd_cgroup)
		g_ptr_array_add(runtime_argv, "--systemd-cgroup");

	if (exec) {
		g_ptr_array_add (runtime_argv, "exec");
		g_ptr_array_add (runtime_argv, "-d");
		g_ptr_array_add (runtime_argv, "--pid-file");
		g_ptr_array_add (runtime_argv, pid_file);
        } else {
		g_ptr_array_add (runtime_argv, "create");
		g_ptr_array_add (runtime_argv, "--bundle");
		g_ptr_array_add (runtime_argv, bundle_path);
		g_ptr_array_add (runtime_argv, "--pid-file");
		g_ptr_array_add (runtime_argv, pid_file);
	}

	if (terminal) {
		g_ptr_array_add(runtime_argv, "--console-socket");
		g_ptr_array_add(runtime_argv, csname);
	}

	/* Set the exec arguments. */
	if (exec) {
		g_ptr_array_add(runtime_argv, "--process");
		g_ptr_array_add(runtime_argv, exec_process_spec);
	}

	/* Container name comes last. */
	g_ptr_array_add(runtime_argv, cid);
	g_ptr_array_add(runtime_argv, NULL);

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
		_cleanup_close_ int dev_null = -1;
		/* FIXME: This results in us not outputting runc error messages to crio's log. */
		if (slavefd_stdin < 0) {
			dev_null = open("/dev/null", O_RDONLY);
			if (dev_null < 0)
				pexit("Failed to open /dev/null");
			slavefd_stdin = dev_null;
		}
		if (dup2(slavefd_stdin, STDIN_FILENO) < 0)
			pexit("Failed to dup over stdout");

		if (slavefd_stdout >= 0) {
			if (dup2(slavefd_stdout, STDOUT_FILENO) < 0)
				pexit("Failed to dup over stdout");
		}
		if (slavefd_stderr >= 0) {
			if (dup2(slavefd_stderr, STDERR_FILENO) < 0)
				pexit("Failed to dup over stderr");
		}

		execv(g_ptr_array_index(runtime_argv,0), (char **)runtime_argv->pdata);
		exit(127);
	}

	g_ptr_array_free (runtime_argv, TRUE);

	/* The runtime has that fd now. We don't need to touch it anymore. */
	close(slavefd_stdin);
	close(slavefd_stdout);
	close(slavefd_stderr);

	ninfo("about to waitpid: %d", create_pid);
	if (terminal) {
		guint terminal_watch = g_unix_fd_add (csfd, G_IO_IN, terminal_accept_cb, csname);
		g_child_watch_add (create_pid, runtime_exit_cb, NULL);
		g_main_loop_run (main_loop);
		g_source_remove (terminal_watch);
	} else {
		/* Wait for our create child to exit with the return code. */
		if (waitpid(create_pid, &runtime_status, 0) < 0) {
			int old_errno = errno;
			kill(create_pid, SIGKILL);
			errno = old_errno;
			pexit("Failed to wait for `runtime %s`", exec ? "exec" : "create");
		}
	}

	if (!WIFEXITED(runtime_status) || WEXITSTATUS(runtime_status) != 0) {
		if (sync_pipe_fd > 0 && !exec) {
			if (terminal) {
				/* 
				 * For this case, the stderr is captured in the parent when terminal is passed down.
			         * We send -1 as pid to signal to parent that create container has failed.
				 */
				len = snprintf(buf, BUF_SIZE, "{\"pid\": %d}\n", -1);
				if (len < 0 || write_all(sync_pipe_fd, buf, len) != len) {
					pexit("unable to send container pid to parent");
				}
			} else {
				/*
				 * Read from container stderr for any error and send it to parent
			         * We send -1 as pid to signal to parent that create container has failed.
				 */
				num_read = read(masterfd_stderr, buf, BUF_SIZE);
				if (num_read > 0) {
					_cleanup_free_ char *escaped_message = NULL;
					ssize_t len;

					buf[num_read] = '\0';
					escaped_message = escape_json_string(buf);

					len = snprintf(buf, BUF_SIZE, "{\"pid\": %d, \"message\": \"%s\"}\n", -1, escaped_message);
					if (len < 0 || write_all(sync_pipe_fd, buf, len) != len) {
						ninfo("Unable to send container stderr message to parent");
					}
				}
			}
		}
		nexit("Failed to create container: exit status %d", WEXITSTATUS(runtime_status));
	}

	if (terminal && masterfd_stdout == -1)
		pexit("Runtime did not set up terminal");

	/* Read the pid so we can wait for the process to exit */
	g_file_get_contents(pid_file, &contents, NULL, &err);
	if (err) {
		nwarn("Failed to read pidfile: %s", err->message);
		g_error_free(err);
		exit(1);
	}

	cpid = atoi(contents);
	ninfo("container PID: %d", cpid);

	/* Setup endpoint for attach */
	char attach_symlink_dir_path[PATH_MAX];
	struct sockaddr_un attach_addr = {0};

	if (!exec) {
		attach_addr.sun_family = AF_UNIX;

		/*
		 * Create a symlink so we don't exceed unix domain socket
		 * path length limit.
		 */
		snprintf(attach_symlink_dir_path, PATH_MAX, "/var/run/crio/%s", cuuid);
		if (unlink(attach_symlink_dir_path) == -1 && errno != ENOENT) {
			pexit("Failed to remove existing symlink for attach socket directory");
		}
		if (symlink(bundle_path, attach_symlink_dir_path) == -1)
			pexit("Failed to create symlink for attach socket");

		snprintf(attach_sock_path, PATH_MAX, "/var/run/crio/%s/attach", cuuid);
		ninfo("attach sock path: %s", attach_sock_path);

		strncpy(attach_addr.sun_path, attach_sock_path, sizeof(attach_addr.sun_path) - 1);
		ninfo("addr{sun_family=AF_UNIX, sun_path=%s}", attach_addr.sun_path);

		/*
		 * We make the socket non-blocking to avoid a race where client aborts connection
		 * before the server gets a chance to call accept. In that scenario, the server
		 * accept blocks till a new client connection comes in.
		 */
		afd = socket(AF_UNIX, SOCK_SEQPACKET|SOCK_NONBLOCK|SOCK_CLOEXEC, 0);
		if (afd == -1)
			pexit("Failed to create attach socket");

                if (fchmod(afd, 0700))
			pexit("Failed to change attach socket permissions");

		if (bind(afd, (struct sockaddr *)&attach_addr, sizeof(struct sockaddr_un)) == -1)
			pexit("Failed to bind attach socket: %s", attach_sock_path);

		if (listen(afd, 10) == -1)
			pexit("Failed to listen on attach socket: %s", attach_sock_path);
	}

	/* Setup fifo for reading in terminal resize and other stdio control messages */
	_cleanup_close_ int ctlfd = -1;
	_cleanup_close_ int dummyfd = -1;
	if (!exec) {
		snprintf(ctl_fifo_path, PATH_MAX, "%s/ctl", bundle_path);
		ninfo("ctl fifo path: %s", ctl_fifo_path);

		if (mkfifo(ctl_fifo_path, 0666) == -1)
			pexit("Failed to mkfifo at %s", ctl_fifo_path);

		ctlfd = open(ctl_fifo_path, O_RDONLY|O_NONBLOCK|O_CLOEXEC);
		if (ctlfd == -1)
			pexit("Failed to open control fifo");

		/*
		 * Open a dummy writer to prevent getting flood of POLLHUPs when
		 * last writer closes.
		 */
		dummyfd = open(ctl_fifo_path, O_WRONLY|O_CLOEXEC);
		if (dummyfd == -1)
			pexit("Failed to open dummy writer for fifo");

		ninfo("ctlfd: %d", ctlfd);
	}

	/* Send the container pid back to parent */
	if (sync_pipe_fd > 0 && !exec) {
		len = snprintf(buf, BUF_SIZE, "{\"pid\": %d}\n", cpid);
		if (len < 0 || write_all(sync_pipe_fd, buf, len) != len) {
			pexit("unable to send container pid to parent");
		}
	}

	/* Setup OOM notification for container process */
	memory_cgroup_path = process_cgroup_subsystem_path(cpid, "memory");
	if (!memory_cgroup_path) {
		nexit("Failed to get memory cgroup path");
	}

	bool oom_handling_enabled = true;
	char memory_cgroup_file_path[PATH_MAX];
	snprintf(memory_cgroup_file_path, PATH_MAX, "%s/cgroup.event_control", memory_cgroup_path);
	if ((cfd = open(memory_cgroup_file_path, O_WRONLY | O_CLOEXEC)) == -1) {
		nwarn("Failed to open %s", memory_cgroup_file_path);
		oom_handling_enabled = false;
	}

	if (oom_handling_enabled) {
		snprintf(memory_cgroup_file_path, PATH_MAX, "%s/memory.oom_control", memory_cgroup_path);
		if ((ofd = open(memory_cgroup_file_path, O_RDONLY | O_CLOEXEC)) == -1)
			pexit("Failed to open %s", memory_cgroup_file_path);

		if ((oom_efd = eventfd(0, EFD_CLOEXEC)) == -1)
			pexit("Failed to create eventfd");

		wb = snprintf(buf, BUF_SIZE, "%d %d", oom_efd, ofd);
		if (write_all(cfd, buf, wb) < 0)
			pexit("Failed to write to cgroup.event_control");
	}

	if (masterfd_stdout >= 0) {
		g_unix_fd_add (masterfd_stdout, G_IO_IN, stdio_cb, GINT_TO_POINTER(STDOUT_PIPE));
		num_stdio_fds++;
	}
	if (masterfd_stderr >= 0) {
		g_unix_fd_add (masterfd_stderr, G_IO_IN, stdio_cb, GINT_TO_POINTER(STDERR_PIPE));
		num_stdio_fds++;
	}

	/* Add the OOM event fd to epoll */
	if (oom_handling_enabled) {
		g_unix_fd_add (oom_efd, G_IO_IN, oom_cb, NULL);
	}

	/* Add the attach socket to epoll */
	if (afd > 0) {
		g_unix_fd_add (afd, G_IO_IN, attach_cb, NULL);
	}

	/* Add control fifo fd to epoll */
	if (ctlfd > 0) {
		g_unix_fd_add (ctlfd, G_IO_IN, ctrl_cb, NULL);
	}

	g_main_loop_run (main_loop);

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
					if (len < 0 || write_all(sync_pipe_fd, buf, len) != len) {
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
		if (len < 0 || write_all(sync_pipe_fd, buf, len) != len) {
			pexit("unable to send exit status");
			exit(1);
		}
	}

	if (!exec) {
		if (unlink(attach_symlink_dir_path) == -1 && errno != ENOENT) {
			pexit("Failed to remove symlink for attach socket directory");
		}
	}

	return EXIT_SUCCESS;
}
