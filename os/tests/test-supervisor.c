/* SPDX-License-Identifier: MIT -- real child/process-group fault tests. */
#define _GNU_SOURCE
#include "supervisor.h"
#include <assert.h>
#include <fcntl.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/prctl.h>
#include <sys/stat.h>
#include <sys/wait.h>
#include <unistd.h>

static int healthy = 1;
static volatile sig_atomic_t stop_requested;
static void request_stop(int signal_number) { (void)signal_number; stop_requested=1; }
static int probe(void) { return healthy; }
static void tick(struct service *s) { service_tick(s); service_reap_orphans(s); usleep(1000); }
static void until(struct service *s, enum service_state state, unsigned ms)
{
    long long end = monotonic_ms() + ms;
    while (s->state != state && monotonic_ms() < end) tick(s);
    if (s->state != state) fprintf(stderr, "Expected %s, got %s\n", service_state_name(state), service_state_name(s->state));
    assert(s->state == state);
}

int main(int argc, char **argv)
{
    if (argc == 3 && !strcmp(argv[1], "--worker")) {
        puts("worker diagnostic"); fflush(stdout);
        if (!strcmp(argv[2], "exit")) return 7;
        if (!strcmp(argv[2], "ordered")) {
            int channel[2]; assert(!pipe(channel));
            pid_t child=fork(); assert(child>=0);
            if (!child) {
                close(channel[1]); char byte;
                if (read(channel[0],&byte,1)!=1) _exit(9);
                _exit(byte=='S'?0:8);
            }
            close(channel[0]); signal(SIGPIPE,SIG_IGN);
            sigset_t blocked, previous_mask;
            sigemptyset(&blocked); sigaddset(&blocked,SIGTERM);
            assert(!sigprocmask(SIG_BLOCK,&blocked,&previous_mask));
            signal(SIGTERM,request_stop);
            service_notify();
            while (!stop_requested) sigsuspend(&previous_mask);
            assert(!sigprocmask(SIG_SETMASK,&previous_mask,NULL));
            assert(write(channel[1],"S",1)==1); close(channel[1]);
            int status; assert(waitpid(child,&status,0)==child);
            assert(WIFEXITED(status) && WEXITSTATUS(status)==0);
            puts("ORDERED_CHILD_CLEANUP_OK"); fflush(stdout);
            return 0;
        }
        if (!strcmp(argv[2], "group")) {
            if (!fork()) { signal(SIGTERM, SIG_IGN); for (;;) pause(); }
        }
        if (!strcmp(argv[2], "flood")) {
            char buffer[4096]; memset(buffer, 'x', sizeof(buffer));
            for (int i = 0; i < 512; i++) if (write(1, buffer, sizeof(buffer)) < 0) return 1;
        }
        for (;;) pause();
    }
    assert(!prctl(PR_SET_CHILD_SUBREAPER, 1));
    char temp[] = "/tmp/fractal-supervisor-XXXXXX";
    assert(mkdtemp(temp));
    char path[256]; snprintf(path, sizeof(path), "%s/service.log", temp);
    char *args[] = {argv[0], "--worker", "exit", NULL};
    struct service s = {.name="test", .argv=args, .log_path=path, .start_ms=100,
        .stop_ms=50, .backoff_ms=5, .healthy_ms=50, .max_failures=4};
    service_init(&s); service_start(&s);
    until(&s, SV_FAILED, 4000);
    assert(s.starts == 4 && !s.pid && !s.group);
    puts("PASS unexpected exits restart with backoff and stop at the failure limit");

    args[2] = "group"; s.probe = probe; healthy = 0;
    service_start(&s);
    until(&s, SV_FAILED, 4000);
    assert(!s.pid && !s.group);
    puts("PASS readiness timeout and SIGKILL cleanup of TERM-resistant descendants");

    args[2] = "wait"; healthy = -1;
    service_start(&s); tick(&s); assert(s.state == SV_STARTING);
    healthy = 1; s.next_probe = 0; until(&s, SV_RUNNING, 1000);
    healthy = -1; s.health_misses = 1;
    for (int i = 0; i < 5; i++) { s.next_probe = 0; tick(&s); }
    assert(s.state == SV_RUNNING && s.health_misses == 1);
    healthy = 1; s.next_probe = 0; tick(&s); assert(!s.health_misses);
    puts("PASS pending probes neither declare readiness nor consume or reset the failure budget");
    healthy = 0;
    for (int i = 0; i < 3; i++) { s.next_probe = 0; tick(&s); }
    assert(s.state == SV_STOPPING || s.state == SV_BACKOFF);
    service_stop(&s); until(&s, SV_STOPPED, 2000);
    unsigned starts = s.starts;
    for (int i = 0; i < 100; i++) tick(&s);
    assert(s.starts == starts);
    puts("PASS health failure detection and explicit stop without automatic restart");

    healthy = 1; args[2] = "flood";
    service_start(&s); until(&s, SV_RUNNING, 1000);
    for (int i = 0; i < 700; i++) tick(&s);
    char previous[260]; snprintf(previous, sizeof(previous), "%s.1", path);
    struct stat a, b;
    assert(!stat(path, &a) && !stat(previous, &b));
    assert(a.st_size <= 262144 && b.st_size <= 262144);
    service_restart(&s); until(&s, SV_BACKOFF, 2000); until(&s, SV_RUNNING, 2000);
    pid_t helper = fork();
    assert(helper >= 0);
    if (!helper) { usleep(150000); _exit(23); }
    service_stop(&s);
    int helper_status = 0, helper_done = 0;
    long long deadline = monotonic_ms() + 2000;
    while ((!helper_done || s.state != SV_STOPPED) && monotonic_ms() < deadline) {
        service_tick(&s);
        if (service_reap_children(&s, helper, &helper_status)) helper_done = 1;
        usleep(1000);
    }
    assert(helper_done && WIFEXITED(helper_status) && WEXITSTATUS(helper_status) == 23);
    assert(s.state == SV_STOPPED);
    puts("PASS bounded log rotation and requested restart");
    args[2]="ordered"; s.notify=1; s.probe=NULL; s.terminate_leader=1; s.stop_ms=1000;
    service_start(&s); until(&s,SV_RUNNING,2000);
    service_stop(&s); until(&s,SV_STOPPED,2000);
    FILE *log=fopen(path,"r"); assert(log);
    char line[4096]; int ordered=0;
    while (fgets(line,sizeof(line),log)) if (strstr(line,"ORDERED_CHILD_CLEANUP_OK")) ordered=1;
    fclose(log); assert(ordered);
    puts("PASS leader-first termination lets daemons coordinate child shutdown before group cleanup");
    unlink(path); unlink(previous); rmdir(temp);
    return 0;
}
