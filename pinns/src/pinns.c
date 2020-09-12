#define _GNU_SOURCE
#include <fcntl.h>
#include <getopt.h>
#include <linux/limits.h>
#include <sched.h>
#include <signal.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <sys/mount.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <unistd.h>
#include <sys/prctl.h>
#include <arpa/inet.h>
#include <sys/wait.h>

#include "utils.h"
#include "sysctl.h"

static int bind_ns(const char *pin_path, const char *filename, const char *ns_name, pid_t pid);
static int directory_exists_or_create(const char* path);

static int write_mapping_file(pid_t pid, const char *mapping, bool is_gidmapping);

enum {
      UID_MAPPING = 1000,
      GID_MAPPING = 1001,
};

int main(int argc, char **argv) {
  const char *uid_mapping = NULL;
  const char *gid_mapping = NULL;
  int num_unshares = 0;
  int unshare_flags = 0;
  int c;
  int p[2];
  pid_t pid;
  char *pin_path = NULL;
  char *filename = NULL;
  bool bind_net = false;
  bool bind_uts = false;
  bool bind_ipc = false;
  bool bind_user = false;
  bool bind_cgroup = false;
  char *sysctls = NULL;

  static const struct option long_options[] = {
      {"help", no_argument, NULL, 'h'},
      {"uts", optional_argument, NULL, 'u'},
      {"ipc", optional_argument, NULL, 'i'},
      {"net", optional_argument, NULL, 'n'},
      {"user", optional_argument, NULL, 'U'},
      {"cgroup", optional_argument, NULL, 'c'},
      {"dir", required_argument, NULL, 'd'},
      {"filename", required_argument, NULL, 'f'},
      {"uid-mapping", optional_argument, NULL, UID_MAPPING},
      {"gid-mapping", optional_argument, NULL, GID_MAPPING},
      {"sysctl", optional_argument, NULL, 's'},
  };

  while ((c = getopt_long(argc, argv, "pchuUind:f:s:", long_options, NULL)) != -1) {
    switch (c) {
    case 'u':
      unshare_flags |= CLONE_NEWUTS;
      bind_uts = true;
      num_unshares++;
      break;
    case 'i':
      unshare_flags |= CLONE_NEWIPC;
      bind_ipc = true;
      num_unshares++;
      break;
    case 'n':
      unshare_flags |= CLONE_NEWNET;
      bind_net = true;
      num_unshares++;
      break;
    case 'U':
      unshare_flags |= CLONE_NEWUSER;
      bind_user = true;
      num_unshares++;
      break;
    case 'c':
#ifdef CLONE_NEWCGROUP
      unshare_flags |= CLONE_NEWCGROUP;
      bind_cgroup = true;
      num_unshares++;
      break;
#endif
      pexit("unsharing cgroups is not supported by this pinns version");
    case 'd':
      pin_path = optarg;
      break;
    case 's':
	  sysctls = optarg;
      break;
    case 'f':
      filename = optarg;
      break;
    case UID_MAPPING:
      uid_mapping = optarg;
      break;
    case GID_MAPPING:
      gid_mapping = optarg;
      break;
    case 'h':
      // usage();
    default:
      // usage();
      return EXIT_FAILURE;
    }
  }

  if (!pin_path) {
    pexit("Path for pinning namespaces not specified");
  }

  if (!filename) {
    pexit("Filename for pinning namespaces not specified");
  }

  if (directory_exists_or_create(pin_path) < 0) {
    nexitf("%s exists but is not a directory", pin_path);
  }

  if (num_unshares == 0) {
    nexit("No namespace specified for pinning");
  }

  if (bind_user && (uid_mapping == NULL || gid_mapping == NULL))
    nexit("Creating new user namespace but mappings not specified");
  if (!bind_user && (uid_mapping != NULL || gid_mapping != NULL))
    nexit("Mappings specified without creating a new user namespace");

  if (!bind_user) {
    /* Use pid=0 to indicate using the current process.  */
    pid = 0;

    if (unshare(unshare_flags) < 0) {
      pexit("Failed to unshare namespaces");
    }
  } else {
    /* if we create a user namespace, we need a new process.  */
    if (pipe2(p, O_DIRECT) < 0)
      pexit("pipe");

    pid = fork();
    if (pid < 0)
      pexit("Failed to fork");

    if (pid == 0) {
      close(p[0]);
      if (prctl(PR_SET_PDEATHSIG, SIGKILL) < 0)
        pexit("Failed to prctl");
      if (unshare(unshare_flags) < 0) {
        pexit("Failed to unshare namespaces");
      }
      if (TEMP_FAILURE_RETRY(write(p[1], "0", 1)) < 0)
        pexit("Failed to write on sync pipe");

      if (TEMP_FAILURE_RETRY(close(p[1]) < 0))
        pexit("Failed to close pipe");

      for (;;)
        pause();
      _exit (EXIT_SUCCESS);
    }
    if (TEMP_FAILURE_RETRY(close(p[1])) < 0)
      pexit("Failed to close pipe");
    /* Namespaces created.  */
    if (TEMP_FAILURE_RETRY(read (p[0], &c, 1)) < 0)
      pexit("Failed to read from the sync pipe");
    close(p[0]);

    if (gid_mapping && write_mapping_file(pid, gid_mapping, true) < 0)
      pexit("Cannot write gid mappings");

    if (uid_mapping && write_mapping_file(pid, uid_mapping, false) < 0)
      pexit("Cannot write gid mappings");
  }

  if (sysctls && configure_sysctls(sysctls) < 0) {
    pexit("Failed to configure sysctls after unshare");
  }

  if (bind_user) {
    if (bind_ns(pin_path, filename, "user", pid) < 0) {
      return EXIT_FAILURE;
    }
  }

  if (bind_uts) {
    if (bind_ns(pin_path, filename, "uts", pid) < 0) {
      return EXIT_FAILURE;
    }
  }

  if (bind_ipc) {
    if (bind_ns(pin_path, filename, "ipc", pid) < 0) {
      return EXIT_FAILURE;
    }
  }

  if (bind_net) {
    if (bind_ns(pin_path, filename, "net", pid) < 0) {
      return EXIT_FAILURE;
    }
  }

  if (bind_cgroup) {
    if (bind_ns(pin_path, filename, "cgroup", pid) < 0) {
      return EXIT_FAILURE;
    }
  }

  /* Avoid creating a zombie.  */
  if (pid > 0 && kill(pid, SIGKILL) == 0)
    waitpid(pid, NULL, 0);

  return EXIT_SUCCESS;
}

static int bind_ns(const char *pin_path, const char *filename, const char *ns_name, pid_t pid) {
  char bind_path[PATH_MAX];
  char ns_path[PATH_MAX];
  int fd;

  // first, verify the /$PATH/$NSns directory exists
  snprintf(bind_path, PATH_MAX - 1, "%s/%sns", pin_path, ns_name);
  if (directory_exists_or_create(bind_path) < 0) {
    pwarnf("%s exists and is not a directory", bind_path);
    return -1;
  }

  // now, get the real path we want
  snprintf(bind_path, PATH_MAX - 1, "%s/%sns/%s", pin_path, ns_name, filename);

  fd = open(bind_path, O_RDONLY | O_CREAT | O_EXCL, 0);
  if (fd < 0) {
    if (fd < 0 && errno != EEXIST) {
      pwarn("Failed to create ns file");
      return -1;
    }
  }
  close(fd);

  if (pid > 0)
    snprintf(ns_path, PATH_MAX - 1, "/proc/%d/ns/%s", pid, ns_name);
  else
    snprintf(ns_path, PATH_MAX - 1, "/proc/self/ns/%s", ns_name);

  if (mount(ns_path, bind_path, NULL, MS_BIND, NULL) < 0) {
    pwarnf("Failed to bind mount ns: %s", ns_path);
    return -1;
  }

  return 0;
}

static int write_mapping_file(pid_t pid, const char *mapping, bool is_gidmapping) {
  const char *fname = is_gidmapping ? "gid_map" : "uid_map";
  ssize_t content_size;
  char *it, *content;
  size_t written;
  char path[64];
  int fd;

  written = snprintf(path, sizeof(path), "/proc/%d/%s", pid, fname);
  if (written >= sizeof(path))
    {
      errno = EOVERFLOW;
      return -1;
    }

  /* make a writeable copy.  */
  content = strdup(mapping);
  if (content == NULL)
    return -1;

  for (it = content; *it; it++) {
    if (*it == '@')
      *it = '\n';
    else if (*it == '-')
      *it = ' ';
  }
  content_size = it - content + 1;

  fd = open(path, O_WRONLY | O_CLOEXEC);
  if (fd < 0) {
    int saved_errno = errno;
    free (content);
    errno = saved_errno;
    return -1;
  }

  if (write(fd, content, content_size) != content_size) {
    int saved_errno = errno;
    free (content);
    close (fd);
    errno = saved_errno;
    return -1;
  }

  free (content);

  return close (fd);
}

static int directory_exists_or_create(const char* path) {
  struct stat sb;
  if (stat(path, &sb) != 0) {
    return mkdir(path, 0755);
  }

  if (!S_ISDIR(sb.st_mode)) {
    return -1;
  }
  return 0;
}
