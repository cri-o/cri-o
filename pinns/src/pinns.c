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

static bool is_host_ns(const char* const optarg);
static int setup_unbindable_bindpath(const char *pin_path, const char *ns_name);
static int create_bind_root(char *bind_root, size_t size, const char *pin_path, const char *ns_name);
static int bind_ns(const char *pin_path, const char *filename, const char *ns_name, pid_t pid);
static int directory_exists_or_create(const char* path);

static int write_mapping_file(pid_t pid, const char *mapping, bool is_gidmapping);

enum {
      UID_MAPPING = 1000,
      GID_MAPPING = 1001,
};

const char* const HOSTNS = "host";

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
  bool bind_mount = false;
  char **sysctls = NULL;
  int sysctls_count = 0;
  char res;

  static const struct option long_options[] = {
      {"help", no_argument, NULL, 'h'},
      {"uts", optional_argument, NULL, 'u'},
      {"ipc", optional_argument, NULL, 'i'},
      {"net", optional_argument, NULL, 'n'},
      {"user", optional_argument, NULL, 'U'},
      {"cgroup", optional_argument, NULL, 'c'},
      {"mnt", optional_argument, NULL, 'm'},
      {"dir", required_argument, NULL, 'd'},
      {"filename", required_argument, NULL, 'f'},
      {"uid-mapping", optional_argument, NULL, UID_MAPPING},
      {"gid-mapping", optional_argument, NULL, GID_MAPPING},
      {"sysctl", optional_argument, NULL, 's'},
  };

  sysctls = calloc(argc/2, sizeof(char *));
  if (UNLIKELY(sysctls == NULL))
      pexit("Failed to calloc");

  while ((c = getopt_long(argc, argv, "mpchuUind:f:s:", long_options, NULL)) != -1) {
    switch (c) {
    case 'u':
      if (!is_host_ns (optarg))
        unshare_flags |= CLONE_NEWUTS;
      bind_uts = true;
      num_unshares++;
      break;
    case 'i':
      if (!is_host_ns (optarg))
        unshare_flags |= CLONE_NEWIPC;
      bind_ipc = true;
      num_unshares++;
      break;
    case 'n':
      if (!is_host_ns (optarg)) {
	    unshare_flags |= CLONE_NEWNET;
      }
      bind_net = true;
      num_unshares++;
      break;
    case 'U':
      if (!is_host_ns (optarg))
        unshare_flags |= CLONE_NEWUSER;
      bind_user = true;
      num_unshares++;
      break;
    case 'c':
#ifdef CLONE_NEWCGROUP
      if (!is_host_ns (optarg))
        unshare_flags |= CLONE_NEWCGROUP;
      bind_cgroup = true;
      num_unshares++;
      break;
#endif
      pexit("unsharing cgroups is not supported by this pinns version");
    case 'm':
      if (!is_host_ns (optarg))
        unshare_flags |= CLONE_NEWNS;
      bind_mount = true;
      num_unshares++;
      break;
    case 'd':
      pin_path = optarg;
      break;
    case 's':
      sysctls[sysctls_count] = optarg;
      sysctls_count++;
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
    nexit("Path for pinning namespaces not specified");
  }

  if (!filename) {
    nexit("Filename for pinning namespaces not specified");
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

  if (!bind_user && !bind_mount) {
    /* Use pid=0 to indicate using the current process.  */
    pid = 0;

    if (unshare(unshare_flags) < 0) {
      pexit("Failed to unshare namespaces");
    }
  } else {
    /* if we create a user or mount namespace, we need a new process. */
    if (socketpair(AF_UNIX, SOCK_SEQPACKET | SOCK_CLOEXEC, 0, p))
      pexit("socketpair");

    pid = fork();
    if (pid < 0)
      pexit("Failed to fork");

    if (pid == 0) {
      close(p[0]);

      if (prctl(PR_SET_PDEATHSIG, SIGKILL) < 0)
        pexit("Failed to prctl");

      if (bind_user) {
        if (unshare(CLONE_NEWUSER) < 0)
          pexit("Failed to unshare namespaces");

        /* Notify that the user namespace is created.  */
        if (TEMP_FAILURE_RETRY(write(p[1], "0", 1)) < 0)
          pexit("Failed to write on sync pipe");

        /* Wait for the mappings to be written.  */
        res = '1';
        if (TEMP_FAILURE_RETRY(read(p[1], &res, 1)) < 0 || res != '0')
          pexit("Failed to read from the sync pipe");

        if (TEMP_FAILURE_RETRY(setresuid(0, 0, 0)) < 0)
          pexit("Failed to setresuid");
        if (TEMP_FAILURE_RETRY(setresgid(0, 0, 0)) < 0)
          pexit("Failed to setresgid");
      }

      /* Now create all the other namespaces that are owned by the correct user.  */
      if (unshare(unshare_flags & ~CLONE_NEWUSER) < 0)
        pexit("Failed to unshare namespaces");

      /* Notify that the namespaces are created.  */
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

    if (bind_user) {
      /* Wait for user namespace creation.  */
      res = '1';
      if (TEMP_FAILURE_RETRY(read(p[0], &res, 1)) < 0 || res != '0')
        pexit("Failed to read from the sync pipe");

      /* Write user mappings */
      if (gid_mapping && write_mapping_file(pid, gid_mapping, true) < 0)
        pexitf("Cannot write gid mappings: %s", gid_mapping);

      if (uid_mapping && write_mapping_file(pid, uid_mapping, false) < 0)
        pexitf("Cannot write uid mappings: %s", uid_mapping);

      /* Notify that the mappings were written.  */
      if (TEMP_FAILURE_RETRY(write(p[0], "0", 1)) < 0)
        pexit("Failed to write on sync pipe");
    }

    /* Wait for non-user namespace creation.  */
    res = '1';
    if (TEMP_FAILURE_RETRY(read(p[0], &res, 1)) < 0 || res != '0')
      pexit("Failed to read from the sync pipe");

    close(p[0]);
  }

  if (sysctls_count != 0 && configure_sysctls(sysctls, sysctls_count) < 0) {
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

  if (bind_mount) {
    const char *ns_name = "mnt";
    if (setup_unbindable_bindpath(pin_path, ns_name) < 0) {
        return EXIT_FAILURE;
    }
    if (bind_ns(pin_path, filename, ns_name, pid) < 0) {
      return EXIT_FAILURE;
    }
  }

  /* Avoid creating a zombie.  */
  if (pid > 0 && kill(pid, SIGKILL) == 0)
    waitpid(pid, NULL, 0);

  return EXIT_SUCCESS;
}

// returns true if the option is equal to 'host'
static bool is_host_ns(const char* const optarg) {
  return optarg && !strcmp (optarg, HOSTNS);
}

/* Mount namespaces can only be bound into unshareable mount namespaces (to
 * avoid infinite loops), so force the pin_path to be a bind-mount to
 * itself and then marked as unshareable. */
static int setup_unbindable_bindpath(const char *pin_path, const char *ns_name) {
  char bind_root[PATH_MAX];

  // first, verify the /$PATH/$NSns directory exists
  if (create_bind_root(bind_root, PATH_MAX - 1, pin_path, ns_name) < 0) {
    return -1;
  }

  // TODO: Check if bind_root is already a mountpoint
  // For now, just blindly try to bindmount itself to itself, ignoring
  // failures.  If this succeeds, we know it's a mountpoint.  If it fails it
  // probably failed because it's already a mountpoint, and if it's not, the
  // call to set MS_UNBINDABLE will fail next.
  mount(bind_root, bind_root, NULL, MS_BIND, NULL);

  // Now that bind_root is definitely a mountpoint, set it to be UNBINDABLE (idempotent-safe)
  if (mount(NULL, bind_root, NULL, MS_UNBINDABLE, NULL) < 0) {
    pwarnf("Could not make %s an unshareable mountpoint", bind_root);
    return -1;
  }
  return 0;
}

static int bind_ns(const char *pin_path, const char *filename, const char *ns_name, pid_t pid) {
  char bind_path[PATH_MAX];
  int bind_path_len;
  char ns_path[PATH_MAX];
  int fd;

  // first, verify the /$PATH/$NSns directory exists
  bind_path_len = create_bind_root(bind_path, PATH_MAX - 1, pin_path, ns_name);
  if (bind_path_len < 0) {
    return -1;
  }

  // now, get the real path we want: /$PATH/$NSns/$FILENAME
  bind_path[bind_path_len++] = '/';
  bind_path[bind_path_len] = '\0';
  strncat(bind_path, filename, PATH_MAX - bind_path_len - 1);

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

// Verify the /$PATH/$NSns directory exists and is a directory, returning the
// expanded path name in bind_root and returning the length of bind_root (or -1
// on error)
static int create_bind_root(char *bind_root, size_t size, const char *pin_path, const char *ns_name) {
  int len = snprintf(bind_root, size, "%s/%sns", pin_path, ns_name);
  if (directory_exists_or_create(bind_root) < 0) {
    pwarnf("%s exists and is not a directory", bind_root);
    return -1;
  }
  return len;
}

