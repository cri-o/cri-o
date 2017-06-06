#pragma once

#include <stdbool.h>

typedef struct _polling_set_t polling_set_t;
struct _polling_set_t {
	int epfd;
	GHashTable *fd_hash;
};

#define POLLING_SET_INIT { -1, NULL }

typedef void (*polling_set_input_cb) (polling_set_t *set, int fd, gpointer user_data);
typedef bool (*polling_set_error_cb) (polling_set_t *set, int fd, gpointer user_data);

int polling_set_init(polling_set_t *set);
void polling_set_destroy(polling_set_t *set);
int polling_set_add_fd(polling_set_t *set, int fd, polling_set_input_cb input_cb, polling_set_error_cb error_cb, gpointer user_data);

int polling_set_iterate(polling_set_t *set);

#define _cleanup_pollset_ __attribute__((cleanup(polling_set_destroy)))
