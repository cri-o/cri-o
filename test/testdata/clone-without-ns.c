#define _GNU_SOURCE
#include <unistd.h>
#include <stdio.h>
#include <stdlib.h>
#include <sched.h>

int entry() {
        system("id");
        return 0;
}

int main() {
        void *stack = malloc(1024*1024);
        if (clone(entry, (stack+1024*1024), 0, 0) == -1) {
          perror("clone");
        }
}