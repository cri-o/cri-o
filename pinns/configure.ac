AC_PREREQ([2.69])
AC_INIT([pinns], [0.01], [peterhunt@keemail.me])
AC_CONFIG_HEADERS([config.h])
AC_CONFIG_MACRO_DIR([m4])
AC_CONFIG_AUX_DIR([build-aux])

AC_CONFIG_SRCDIR([src/pinns.c])

AC_PROG_RANLIB
LT_INIT([disable-shared])

AM_INIT_AUTOMAKE([1.11 -Wno-portability foreign tar-ustar no-dist-gzip dist-xz subdir-objects])

AM_MAINTAINER_MODE([enable])
AM_SILENT_RULES([yes])

AC_PROG_CC

AC_SEARCH_LIBS(yajl_tree_get, [yajl], [AC_DEFINE([HAVE_YAJL], 1, [Define if libyajl is available])], [AC_MSG_ERROR([*** libyajl headers not found])])
PKG_CHECK_MODULES([YAJL], [yajl >= 2.0.0])

AC_CONFIG_FILES([Makefile])

dnl libsystemd
AC_ARG_ENABLE([systemd],
	AS_HELP_STRING([--disable-systemd], [Ignore systemd and disable support]))
AS_IF([test "x$enable_systemd" != "xno"], [
	AC_CHECK_HEADERS([systemd/sd-bus.h], [], [AC_MSG_ERROR([*** Missing libsystemd headers])])
	AS_IF([test "$ac_cv_header_systemd_sd_bus_h" = "yes"], [
		AC_SEARCH_LIBS(sd_listen_fds, [systemd], [AC_DEFINE([HAVE_SYSTEMD], 1, [Define if libsystemd is available])], [AC_MSG_ERROR([*** Failed to find libsystemd])])
	])
])

dnl libcap
AC_ARG_ENABLE([caps],
	AS_HELP_STRING([--disable-caps], [Ignore libcap and disable support]))
AS_IF([test "x$enable_caps" != "xno"], [
	AC_CHECK_HEADERS([sys/capability.h], [], [AC_MSG_ERROR([*** POSIX caps headers not found])])
	AS_IF([test "$ac_cv_header_sys_capability_h" = "yes"], [
		AC_SEARCH_LIBS(cap_from_name, [cap], [AC_DEFINE([HAVE_CAP], 1, [Define if libcap is available])], [AC_MSG_ERROR([*** libcap headers not found])])
	])
])

AC_CONFIG_SUBDIRS([libocispec])
AC_OUTPUT
