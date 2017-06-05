#define _GNU_SOURCE

#include <glib.h>
#include <sys/epoll.h>
#include <unistd.h>
#include <errno.h>

#include "pollset.h"

#define MAX_EVENTS 10

typedef struct _polling_fd_t polling_fd_t;
struct _polling_fd_t {
	int fd;
	polling_set_input_cb input_cb;
	polling_set_error_cb error_cb;
	gpointer user_data;
};

static void polling_fd_free(polling_fd_t *polling_fd)
{
        if (polling_fd->fd >= 0)
		close(polling_fd->fd);
	g_free(polling_fd);
}

int polling_set_init(polling_set_t *set)
{
	set->epfd = epoll_create1(EPOLL_CLOEXEC);
	if (set->epfd < 0)
		return -1;
	set->fd_hash = g_hash_table_new_full(g_direct_hash, g_direct_equal,
					     NULL, (GDestroyNotify)polling_fd_free);
	return 0;
}

void polling_set_destroy(polling_set_t *set)
{
	if (set->epfd >= 0)
		close(set->epfd);
	if (set->fd_hash)
		g_hash_table_destroy(set->fd_hash);
}

int polling_set_add_fd(polling_set_t *set, int fd, polling_set_input_cb input_cb, polling_set_error_cb error_cb, gpointer user_data)
{
	struct epoll_event ev;
	polling_fd_t *polling_fd;
	
	ev.events = EPOLLIN;
	ev.data.fd = fd;

	if (epoll_ctl(set->epfd, EPOLL_CTL_ADD, ev.data.fd, &ev) < 0)
		return -1;

	polling_fd = g_new0(polling_fd_t, 1);
	polling_fd->fd = fd;
	polling_fd->input_cb = input_cb;
	polling_fd->error_cb = error_cb;
	polling_fd->user_data = user_data;

	g_hash_table_insert(set->fd_hash, GINT_TO_POINTER(fd), polling_fd);
	return 0;
}

void polling_set_remove_fd(polling_set_t *set, int fd)
{
	g_hash_table_remove(set->fd_hash, GINT_TO_POINTER(fd));
}

int polling_set_iterate(polling_set_t *set)
{
	struct epoll_event evlist[MAX_EVENTS];
	int ready;

	do {
		ready = epoll_wait(set->epfd, evlist, MAX_EVENTS, -1);
	} while (ready == -1 && errno == EINTR);

	if (ready < 0)
		return -1;

	for (int i = 0; i < ready; i++) {
		int fd = evlist[i].data.fd;
		polling_fd_t *polling_fd = g_hash_table_lookup(set->fd_hash, GINT_TO_POINTER(fd));

		if (polling_fd == NULL)
			continue;

		if (evlist[i].events & EPOLLIN &&
		    polling_fd->input_cb)
			polling_fd->input_cb(set, fd, polling_fd->user_data);
		else if (evlist[i].events & (EPOLLHUP | EPOLLERR)) {
			bool remove = true;
			if (polling_fd->error_cb)
				remove = polling_fd->error_cb(set, fd, polling_fd->user_data);
			if (remove)
				polling_set_remove_fd(set, fd);
		}
  	}

	return 0;
}
