#pragma once
#if !defined(UTILS_H)
#define UTILS_H

#include <stdio.h>
#include <syslog.h>
#include <stdbool.h>
#include <unistd.h>
#include <glib.h>
#include <glib-unix.h>
#include <sys/uio.h>

/* stdpipe_t represents one of the std pipes (or NONE).
 * Sync with const in container_attach.go */
typedef enum {
	NO_PIPE,
	STDIN_PIPE, /* unused */
	STDOUT_PIPE,
	STDERR_PIPE,
} stdpipe_t;

/* Different levels of logging */
typedef enum {
	EXIT_LEVEL,
	WARN_LEVEL,
	INFO_LEVEL,
	DEBUG_LEVEL,
} log_level_t;

// Default log level is Warning, This will be configured before any logging
// should happen
extern log_level_t log_level;
extern char *cid;
extern gboolean use_syslog;

#define pexit(s) \
	do { \
		fprintf(stderr, "[conmon:e]: %s %s\n", s, strerror(errno)); \
		if (use_syslog) \
			syslog(LOG_ERR, "conmon %.20s <error>: %s %s\n", cid, s, strerror(errno)); \
		exit(EXIT_FAILURE); \
	} while (0)

#define pexitf(fmt, ...) \
	do { \
		fprintf(stderr, "[conmon:e]: " fmt " %s\n", ##__VA_ARGS__, strerror(errno)); \
		if (use_syslog) \
			syslog(LOG_ERR, "conmon %.20s <error>: " fmt ": %s\n", cid, ##__VA_ARGS__, strerror(errno)); \
		exit(EXIT_FAILURE); \
	} while (0)

#define pwarn(s) \
	do { \
		fprintf(stderr, "[conmon:w]: %s %s\n", s, strerror(errno)); \
		if (use_syslog) \
			syslog(LOG_INFO, "conmon %.20s <pwarn>: %s %s\n", cid, s, strerror(errno)); \
	} while (0)

#define nexit(s) \
	do { \
		fprintf(stderr, "[conmon:e] %s\n", s); \
		if (use_syslog) \
			syslog(LOG_ERR, "conmon %.20s <error>: %s\n", cid, s); \
		exit(EXIT_FAILURE); \
	} while (0)

#define nexitf(fmt, ...) \
	do { \
		fprintf(stderr, "[conmon:e]: " fmt "\n", ##__VA_ARGS__); \
		if (use_syslog) \
			syslog(LOG_ERR, "conmon %.20s <error>: " fmt " \n", cid, ##__VA_ARGS__); \
		exit(EXIT_FAILURE); \
	} while (0)

#define nwarn(s) \
	if (log_level >= WARN_LEVEL) { \
		do { \
			fprintf(stderr, "[conmon:w]: %s\n", s); \
			if (use_syslog) \
				syslog(LOG_INFO, "conmon %.20s <nwarn>: %s\n", cid, s); \
		} while (0); \
	}

#define nwarnf(fmt, ...) \
	if (log_level >= WARN_LEVEL) { \
		do { \
			fprintf(stderr, "[conmon:w]: " fmt "\n", ##__VA_ARGS__); \
			if (use_syslog) \
				syslog(LOG_INFO, "conmon %.20s <nwarn>: " fmt " \n", cid, ##__VA_ARGS__); \
		} while (0); \
	}

#define ninfo(s) \
	if (log_level >= INFO_LEVEL) { \
		do { \
			fprintf(stderr, "[conmon:i]: %s\n", s); \
			if (use_syslog) \
				syslog(LOG_INFO, "conmon %.20s <ninfo>: %s\n", cid, s); \
		} while (0); \
	}

#define ninfof(fmt, ...) \
	if (log_level >= INFO_LEVEL) { \
		do { \
			fprintf(stderr, "[conmon:i]: " fmt "\n", ##__VA_ARGS__); \
			if (use_syslog) \
				syslog(LOG_INFO, "conmon %.20s <ninfo>: " fmt " \n", cid, ##__VA_ARGS__); \
		} while (0); \
	}

#define ndebug(s) \
	if (log_level >= DEBUG_LEVEL) { \
		do { \
			fprintf(stderr, "[conmon:d]: %s\n", s); \
			if (use_syslog) \
				syslog(LOG_INFO, "conmon %.20s <ndebug>: %s\n", cid, s); \
		} while (0); \
	}

#define ndebugf(fmt, ...) \
	if (log_level >= DEBUG_LEVEL) { \
		do { \
			fprintf(stderr, "[conmon:d]: " fmt "\n", ##__VA_ARGS__); \
			if (use_syslog) \
				syslog(LOG_INFO, "conmon %.20s <ndebug>: " fmt " \n", cid, ##__VA_ARGS__); \
		} while (0); \
	}

/* Set the log level for this call. log level defaults to warning.
   parse the string value of level_name to the appropriate log_level_t enum value
*/
void set_conmon_logs(char *level_name, char *cid_, gboolean syslog_);

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

static inline void fclosep(FILE **fp)
{
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
		g_strfreev(*strv);
}

#define _cleanup_free_ _cleanup_(freep)
#define _cleanup_close_ _cleanup_(closep)
#define _cleanup_fclose_ _cleanup_(fclosep)
#define _cleanup_gstring_ _cleanup_(gstring_free_cleanup)
#define _cleanup_strv_ _cleanup_(strv_cleanup)


#define WRITEV_BUFFER_N_IOV 128

typedef struct {
	int iovcnt;
	struct iovec iov[WRITEV_BUFFER_N_IOV];
} writev_buffer_t;


#endif /* !defined(UTILS_H) */
