#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/prctl.h>
#include <sys/wait.h>
#include <errno.h>

#include <glib.h>

#define pexit(fmt, ...)                                                \
	do {                                                           \
		fprintf(stderr, "conmon: " fmt " %m\n", ##__VA_ARGS__); \
		exit(EXIT_FAILURE);                                    \
	} while (0)                                                   

#define nexit(fmt, ...)                                                \
	do {                                                           \
		fprintf(stderr, "conmon: " fmt "\n", ##__VA_ARGS__); \
		exit(EXIT_FAILURE);                                    \
	} while (0)                                                   

#define nexit(fmt, ...)                                                \
	do {                                                           \
		fprintf(stderr, "conmon: " fmt "\n", ##__VA_ARGS__); \
		exit(EXIT_FAILURE);                                    \
	} while (0)                                                   

#define _cleanup_(x) __attribute__((cleanup(x)))

static inline void freep(void *p) {
	free(*(void**) p);
}

#define _cleanup_free_ _cleanup_(freep)

int main(int argc, char *argv[])
{
	int ret;
	const char *cid;
	char cmd[128];
	GError *err = NULL;
	gchar *contents;
	int cpid = -1;
	int status;
	pid_t pid;

	if (argc < 2) {
		nexit("Run as: conmon <id>");
	}

	// Get the container id
	cid = argv[1];

	// Set self as subreaper so we can wait for container process
	// and return its exit code.
	ret = prctl(PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0);
	if (ret != 0) {
		pexit("Failed to set as subreaper");
	}

	snprintf(cmd, 128, "runc create %s --pid-file pidfile", cid);
	ret = system(cmd);
	if (ret != 0) {
		nexit("Failed to create container");
	}

	// Read the pid so we can wait for the process to exit
	g_file_get_contents("pidfile", &contents, NULL, &err);
	if (err) {
		fprintf(stderr, "Failed to read pidfile: %s\n", err->message);
		g_error_free(err);
		exit(1);
	} 

	cpid = atoi(contents);
	printf("container PID: %d\n", cpid);

	while ((pid = waitpid(-1, &status, 0)) > 0) {
		printf("PID %d exited\n", pid);
		if (pid == cpid) {
			_cleanup_free_ char *status_str = NULL;
			ret = asprintf(&status_str, "%d", status);
			if (ret < 0) {
				pexit("Failed to allocate memory for status");
			}
			g_file_set_contents("exit", status_str, strlen(status_str), &err);
			if (err) {
				fprintf(stderr, "Failed to write %s to exit file: %s\n", status_str, err->message);
				g_error_free(err);
				exit(1);
			}
			break;
		}
	}

	return EXIT_SUCCESS;
}
