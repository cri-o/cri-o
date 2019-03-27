#define _GNU_SOURCE
#include "ctr_logging.h"
#include <string.h>

// if the systemd development files were found, we can log to systemd
#ifdef USE_JOURNALD
#include <systemd/sd-journal.h>
#else
// this function should never be used, as journald logging is disabled and
// parsing code errors if USE_JOURNALD isn't flagged.
// This is just to make the compiler happy and the other code prettier
static inline int sd_journal_send(char *fmt, ...)
{
	perror(fmt);
	return -1;
}

#endif

/* strlen("1997-03-25T13:20:42.999999999+01:00 stdout ") + 1 */
#define TSBUFLEN 44


/* Different types of container logging */
static gboolean use_journald_logging = FALSE;
static gboolean use_k8s_logging = FALSE;

/* Value the user must input for each log driver */
static const char *const K8S_FILE_STRING = "k8s-file";
static const char *const JOURNALD_FILE_STRING = "journald";

/* Max log size for any log file types */
static int64_t log_size_max = -1;

/* k8s log file parameters */
static int k8s_log_fd = -1;
static char *k8s_log_path = NULL;

/* journald log file parameters */
static char *cuuid = NULL;

static void parse_log_path(char *log_config);
static const char *stdpipe_name(stdpipe_t pipe);
static int write_journald(int pipe, char *buf, ssize_t num_read);
static int write_k8s_log(stdpipe_t pipe, const char *buf, ssize_t buflen);
static ssize_t writev_buffer_append_segment(int fd, writev_buffer_t *buf, const void *data, ssize_t len);
static ssize_t writev_buffer_flush(int fd, writev_buffer_t *buf);
static int set_k8s_timestamp(char *buf, ssize_t buflen, const char *pipename);
static void reopen_k8s_file(void);


/* configures container log specific information, such as the drivers the user
 * called with and the max log size for log file types. For the log file types
 * (currently just k8s log file), it will also open the log_fd for that specific
 * log file.
 */
void configure_log_drivers(gchar **log_drivers, int64_t log_size_max_, char *cuuid_)
{
	log_size_max = log_size_max_;
	if (log_drivers == NULL)
		nexit("Log driver not provided. Use --log-path");
	for (int driver = 0; log_drivers[driver]; ++driver) {
		parse_log_path(log_drivers[driver]);
	}
	if (use_k8s_logging) {
		/* Open the log path file. */
		k8s_log_fd = open(k8s_log_path, O_WRONLY | O_APPEND | O_CREAT | O_CLOEXEC, 0600);
		if (k8s_log_fd < 0)
			pexit("Failed to open log file");
	}

	if (use_journald_logging) {
#ifndef USE_JOURNALD
		nexit("Include journald in compilation path to log to systemd journal");
#endif
		cuuid = cuuid_;
	}
}


/* parse_log_path branches on log driver type the user inputted.
 * log_config will either be a ':' delimited string containing:
 * <DRIVER_NAME>:<PATH_NAME> or <PATH_NAME>
 * in the case of no colon, the driver will be kubernetes-log-file,
 * in the case the log driver is 'journald', the <PATH_NAME> is ignored.
 * exits with error if <DRIVER_NAME> isn't 'journald' or 'kubernetes-log-file'
 */
static void parse_log_path(char *log_config)
{
	char *driver = strtok(log_config, ":");
	char *path = strtok(NULL, ":");
	if (!strcmp(driver, JOURNALD_FILE_STRING)) {
		use_journald_logging = TRUE;
		return;
	}
	use_k8s_logging = TRUE;
	// If no : was found, driver is the log path, and the driver is
	// kubernetes-log-file, set variables appropriately
	if (path == NULL) {
		k8s_log_path = driver;
	} else if (!strcmp(driver, K8S_FILE_STRING)) {
		k8s_log_path = path;
	} else {
		nexitf("No such log driver %s", driver);
	}
}

/* write container output to all logs the user defined */
bool write_to_logs(stdpipe_t pipe, char *buf, ssize_t num_read)
{
	if (use_k8s_logging && write_k8s_log(pipe, buf, num_read) < 0) {
		nwarn("write_k8s_log failed");
		return G_SOURCE_CONTINUE;
	}
	if (use_journald_logging && write_journald(pipe, buf, num_read) < 0) {
		nwarn("write_journald failed");
		return G_SOURCE_CONTINUE;
	}
	return true;
}

/* write to systemd journal. If the pipe is stdout, write with notice priority,
 * otherwise, write with error priority
 * note: SIZEOF(buf) MUST be greater than num_read
 */
int write_journald(int pipe, char *buf, ssize_t num_read)
{
	// Always null terminate the buffer, just in case.
	buf[num_read] = '\0';
	int message_priority = LOG_NOTICE;
	if (pipe == STDERR_PIPE)
		message_priority = LOG_ERR;
	sd_journal_send("MESSAGE=%s", buf, "PRIORITY=%i", message_priority, "MESSAGE_ID=%s", cuuid, NULL);
	return 0;
}

/*
 * The CRI requires us to write logs with a (timestamp, stream, line) format
 * for every newline-separated line. write_k8s_log writes said format for every
 * line in buf, and will partially write the final line of the log if buf is
 * not terminated by a newline.
 */
static int write_k8s_log(stdpipe_t pipe, const char *buf, ssize_t buflen)
{
	char tsbuf[TSBUFLEN];
	writev_buffer_t bufv = {0};
	static int64_t bytes_written = 0;
	int64_t bytes_to_be_written = 0;

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
		bool partial = FALSE;

		/* Find the end of the line, or alternatively the end of the buffer. */
		line_end = memchr(buf, '\n', buflen);
		if (line_end == NULL) {
			line_end = &buf[buflen - 1];
			partial = TRUE;
		}
		line_len = line_end - buf + 1;

		/* This is line_len bytes + TSBUFLEN - 1 + 2 (- 1 is for ignoring \0). */
		bytes_to_be_written = line_len + TSBUFLEN + 1;

		/* If partial, then we add a \n */
		if (partial) {
			bytes_to_be_written += 1;
		}

		/*
		 * We re-open the log file if writing out the bytes will exceed the max
		 * log size. We also reset the state so that the new file is started with
		 * a timestamp.
		 */
		if ((log_size_max > 0) && (bytes_written + bytes_to_be_written) > log_size_max) {
			bytes_written = 0;

			if (writev_buffer_flush(k8s_log_fd, &bufv) < 0) {
				nwarn("failed to flush buffer to log");
				/*
				 * We are going to reopen the file anyway, in case of
				 * errors discard all we have in the buffer.
				 */
				bufv.iovcnt = 0;
			}
			reopen_k8s_file();
		}

		/* Output the timestamp */
		if (writev_buffer_append_segment(k8s_log_fd, &bufv, tsbuf, TSBUFLEN - 1) < 0) {
			nwarn("failed to write (timestamp, stream) to log");
			goto next;
		}

		/* Output log tag for partial or newline */
		if (partial) {
			if (writev_buffer_append_segment(k8s_log_fd, &bufv, "P ", 2) < 0) {
				nwarn("failed to write partial log tag");
				goto next;
			}
		} else {
			if (writev_buffer_append_segment(k8s_log_fd, &bufv, "F ", 2) < 0) {
				nwarn("failed to write end log tag");
				goto next;
			}
		}

		/* Output the actual contents. */
		if (writev_buffer_append_segment(k8s_log_fd, &bufv, buf, line_len) < 0) {
			nwarn("failed to write buffer to log");
			goto next;
		}

		/* Output a newline for partial */
		if (partial) {
			if (writev_buffer_append_segment(k8s_log_fd, &bufv, "\n", 1) < 0) {
				nwarn("failed to write newline to log");
				goto next;
			}
		}

		bytes_written += bytes_to_be_written;
	next:
		/* Update the head of the buffer remaining to output. */
		buf += line_len;
		buflen -= line_len;
	}

	if (writev_buffer_flush(k8s_log_fd, &bufv) < 0) {
		nwarn("failed to flush buffer to log");
	}

	return 0;
}

static ssize_t writev_buffer_flush(int fd, writev_buffer_t *buf)
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
			iov->iov_base += from_this;
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

	if (buf->iovcnt == WRITEV_BUFFER_N_IOV && writev_buffer_flush(fd, buf) < 0)
		return -1;

	if (len > 0) {
		buf->iov[buf->iovcnt].iov_base = (void *)data;
		buf->iov[buf->iovcnt].iov_len = (size_t)len;
		buf->iovcnt++;
	}

	return 1;
}


static const char *stdpipe_name(stdpipe_t pipe)
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


static int set_k8s_timestamp(char *buf, ssize_t buflen, const char *pipename)
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


	off = (int)tm->tm_gmtoff;
	if (tm->tm_gmtoff < 0) {
		off_sign = '-';
		off = -off;
	}

	len = snprintf(buf, buflen, "%d-%02d-%02dT%02d:%02d:%02d.%09ld%c%02d:%02d %s ", tm->tm_year + 1900, tm->tm_mon + 1, tm->tm_mday,
		       tm->tm_hour, tm->tm_min, tm->tm_sec, ts.tv_nsec, off_sign, off / 3600, (off % 3600) / 60, pipename);

	if (len < buflen)
		err = 0;
	return err;
}

/* reopen all log files */
void reopen_log_files(void)
{
	reopen_k8s_file();
}

/* reopen the k8s log file fd.  */
static void reopen_k8s_file(void)
{
	if (!use_k8s_logging)
		return;

	_cleanup_free_ char *k8s_log_path_tmp = g_strdup_printf("%s.tmp", k8s_log_path);

	/* Sync the logs to disk */
	if (fsync(k8s_log_fd) < 0) {
		pwarn("Failed to sync log file on reopen");
	}

	/* Close the current k8s_log_fd */
	close(k8s_log_fd);

	/* Open the log path file again */
	k8s_log_fd = open(k8s_log_path_tmp, O_WRONLY | O_TRUNC | O_CREAT | O_CLOEXEC, 0600);
	if (k8s_log_fd < 0)
		pexitf("Failed to open log file %s", k8s_log_path);

	/* Replace the previous file */
	if (rename(k8s_log_path_tmp, k8s_log_path) < 0) {
		pexit("Failed to rename log file");
	}
}


void sync_logs(void)
{
	/* Sync the logs to disk */
	if (k8s_log_fd > 0) {
		if (fsync(k8s_log_fd) < 0) {
			pwarn("Failed to sync log file before exit");
		}
	}
}
