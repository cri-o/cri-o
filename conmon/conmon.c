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
#include <sys/eventfd.h>
#include <sys/stat.h>
#include <sys/uio.h>
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
static char *cid = NULL;
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
  { "cid", 'c', 0, G_OPTION_ARG_STRING, &cid, "Container ID", NULL },
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

/* stdpipe_t represents one of the std pipes (or NONE). */
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
	_cleanup_close_ int masterfd_stdout = -1;
	_cleanup_close_ int masterfd_stderr = -1;
	_cleanup_close_ int epfd = -1;
	_cleanup_close_ int csfd = -1;
	/* Used for !terminal cases. */
	int slavefd_stdout = -1;
	int slavefd_stderr = -1;
	char csname[PATH_MAX] = "/tmp/conmon-term.XXXXXXXX";
	char buf[BUF_SIZE];
	int num_read;
	struct epoll_event ev;
	struct epoll_event evlist[MAX_EVENTS];
	int sync_pipe_fd = -1;
	char *sync_pipe, *endptr;
	int len;
	int num_stdio_fds = 0;
	GError *error = NULL;
	GOptionContext *context;
        GPtrArray *runtime_argv = NULL;

	/* Used for OOM notification API */
	_cleanup_close_ int efd = -1;
	_cleanup_close_ int cfd = -1;
	_cleanup_close_ int ofd = -1;
	_cleanup_free_ char *memory_cgroup_path = NULL;
	int wb;
	uint64_t oom_event;

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
		/* We only need to touch the stdio if we have terminal=false. */
		/* FIXME: This results in us not outputting runc error messages to crio's log. */
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
	close(slavefd_stdout);
	close(slavefd_stderr);

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

		/* We only have a single fd for both pipes, so we just treat it as
		 * stdout. stderr is ignored. */
		masterfd_stdout = console.fd;
		masterfd_stderr = -1;

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

		if ((efd = eventfd(0, EFD_CLOEXEC)) == -1)
			pexit("Failed to create eventfd");

		wb = snprintf(buf, BUF_SIZE, "%d %d", efd, ofd);
		if (write_all(cfd, buf, wb) < 0)
			pexit("Failed to write to cgroup.event_control");
	}

	/* Create epoll_ctl so that we can handle read/write events. */
	/*
	 * TODO: Switch to libuv so that we can also implement exec as well as
	 *       attach and other important things. Using epoll directly is just
	 *       really nasty.
	 */
	epfd = epoll_create1(EPOLL_CLOEXEC);
	if (epfd < 0)
		pexit("epoll_create");
	ev.events = EPOLLIN;
	if (masterfd_stdout >= 0) {
		ev.data.fd = masterfd_stdout;
		if (epoll_ctl(epfd, EPOLL_CTL_ADD, ev.data.fd, &ev) < 0)
			pexit("Failed to add console masterfd_stdout to epoll");
		num_stdio_fds++;
	}
	if (masterfd_stderr >= 0) {
		ev.data.fd = masterfd_stderr;
		if (epoll_ctl(epfd, EPOLL_CTL_ADD, ev.data.fd, &ev) < 0)
			pexit("Failed to add console masterfd_stderr to epoll");
		num_stdio_fds++;
	}

	/* Add the OOM event fd to epoll */
	if (oom_handling_enabled) {
		ev.data.fd = efd;
		if (epoll_ctl(epfd, EPOLL_CTL_ADD, ev.data.fd, &ev) < 0)
			pexit("Failed to add OOM eventfd to epoll");
	}

	/* Log all of the container's output. */
	while (num_stdio_fds > 0) {
		int ready = epoll_wait(epfd, evlist, MAX_EVENTS, -1);
		if (ready < 0)
			continue;

		for (int i = 0; i < ready; i++) {
			if (evlist[i].events & EPOLLIN) {
				int masterfd = evlist[i].data.fd;
				stdpipe_t pipe = NO_PIPE;
				if (masterfd == masterfd_stdout)
					pipe = STDOUT_PIPE;
				else if (masterfd == masterfd_stderr)
					pipe = STDERR_PIPE;
				else if (oom_handling_enabled && masterfd == efd) {
					if (read(efd, &oom_event, sizeof(uint64_t)) != sizeof(uint64_t))
						nwarn("Failed to read event from eventfd");
					ninfo("OOM received");
					if (open("oom", O_CREAT, 0666) < 0) {
						nwarn("Failed to write oom file");
					}
				}
				else {
					nwarn("unknown pipe fd");
					goto out;
				}

				if (masterfd == masterfd_stdout || masterfd == masterfd_stderr) {
					num_read = read(masterfd, buf, BUF_SIZE);
					if (num_read <= 0)
						goto out;

					if (write_k8s_log(logfd, pipe, buf, num_read) < 0) {
						nwarn("write_k8s_log failed");
						goto out;
					}
				}
			} else if (evlist[i].events & (EPOLLHUP | EPOLLERR)) {
				printf("closing fd %d\n", evlist[i].data.fd);
				if (close(evlist[i].data.fd) < 0)
					pexit("close");
				num_stdio_fds--;
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

	return EXIT_SUCCESS;
}
