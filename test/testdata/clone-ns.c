#define _GNU_SOURCE
#include <unistd.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sched.h>

int entry() {
    system("id");
    return 0;
}

int main(int argc, char *argv[]) {
    void *stack = malloc(1024*1024);
    int flag;

    if (argc != 2) {
        perror("argument required, 'with_flags' or 'without_flags'");
    }

    if (strcmp(argv[1], "with_flags") == 0) {
        flag = CLONE_NEWUSER|CLONE_NEWNET;
    } else if (strcmp(argv[1], "without_flags") == 0) {
        flag = 0;
    } else {
        perror("invalid argument");
    }

    if (clone(entry, (stack+1024*1024), flag, 0) == -1) {
        perror("clone");
    }
}
