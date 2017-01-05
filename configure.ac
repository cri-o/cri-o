AC_INIT([cri-o], [0.1])
AM_INIT_AUTOMAKE([-Wall foreign subdir-objects])

AC_PROG_CC
AC_CHECK_PROG([GO], [go], [go])

AC_ARG_WITH([travis],
	    [AS_HELP_STRING([--with-travis],
			    [support for Travis CI])],
			    [],
			    [with_travis=no])
AM_CONDITIONAL([TRAVIS], [test "x$with_travis" != xno])

PKG_CHECK_MODULES([glib], [glib-2.0])
PKG_CHECK_MODULES([devmapper], [devmapper])
AC_CHECK_HEADERS([btrfs/ioctl.h], [], [])

AC_ARG_WITH([selinux],
	    [AS_HELP_STRING([--with-selinux],
			    [enable SELinux support])],
			    [],
			    [with_selinux=yes])
AM_CONDITIONAL([USE_SELINUX], [test "x$with_selinux" != xno])

AC_ARG_WITH([seccomp],
	    [AS_HELP_STRING([--with-seccomp],
			    [enable seccomp support])],
			    [],
			    [with_seccomp=yes])
AS_CASE(["$with_seccomp"],
	[yes], [PKG_CHECK_MODULES([libseccomp], [libseccomp])],
	[no], []
	[PKG_CHECK_MODULES([libseccomp], [libseccomp], [have_seccomp=yes], [have_seccomp=no])])
AM_CONDITIONAL([USE_SECCOMP], [test "x$with_seccomp" != xno -a "x$have_seccomp" != xno])

AC_ARG_WITH([apparmor],
	    [AS_HELP_STRING([--with-apparmor],
			    [enable apparmor support])],
			    [],
			    [with_apparmor=no])
AS_CASE(["$with_apparmor"],
	[yes], [PKG_CHECK_MODULES([libapparmor], [libapparmor])],
	[no], []
	[PKG_CHECK_MODULES([libapparmor], [libapparmor], [have_apparmor=yes], [have_apparmor=no])])
AM_CONDITIONAL([USE_APPARMOR], [test "x$with_apparmor" != xno -a "x$have_apparmor" != xno])

CFLAGS="$CFLAGS -Wall -Wextra $glib_CFLAGS"
LDFLAGS="$LDFLAGS"
LIBS="$LIBS $glib_LIBS"

AC_CONFIG_FILES([Makefile conmon/Makefile])

AC_OUTPUT
