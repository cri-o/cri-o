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

#include "utils.h"
#include "sysctl.h"

static int bind_ns(const char *pin_path, const char *filename, const char *ns_name);
static int directory_exists_or_create(const char* path);

int main(int argc, char **argv) {
  int num_unshares = 0;
  int unshare_flags = 0;
  int c;
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

  if (unshare(unshare_flags) < 0) {
    pexit("Failed to unshare namespaces");
  }

  if (sysctls && configure_sysctls(sysctls) < 0) {
    pexit("Failed to configure sysctls after unshare");
  }

  if (bind_uts) {
    if (bind_ns(pin_path, filename, "uts") < 0) {
      return EXIT_FAILURE;
    }
  }

  if (bind_ipc) {
    if (bind_ns(pin_path, filename, "ipc") < 0) {
      return EXIT_FAILURE;
    }
  }

  if (bind_net) {
    if (bind_ns(pin_path, filename, "net") < 0) {
      return EXIT_FAILURE;
    }
  }

  if (bind_user) {
    if (bind_ns(pin_path, filename, "user") < 0) {
      return EXIT_FAILURE;
    }
  }

  if (bind_cgroup) {
    if (bind_ns(pin_path, filename, "cgroup") < 0) {
      return EXIT_FAILURE;
    }
  }

  return EXIT_SUCCESS;
}

static int bind_ns(const char *pin_path, const char *filename, const char *ns_name) {
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
    pwarn("Failed to create ns file");
    return -1;
  }
  close(fd);

  snprintf(ns_path, PATH_MAX - 1, "/proc/self/ns/%s", ns_name);
  if (mount(ns_path, bind_path, NULL, MS_BIND, NULL) < 0) {
    pwarnf("Failed to bind mount ns: %s", ns_path);
    return -1;
  }

  return 0;
}

static int directory_exists_or_create(const char* path) {
  struct stat sb;
  if (stat(path, &sb) != 0) {
    mkdir(path, 0755);
	return 0;
  }

  if (!S_ISDIR(sb.st_mode)) {
    return -1;
  }
  return 0;
}
