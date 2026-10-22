#define _GNU_SOURCE
#include <assert.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <sys/wait.h>
#include <unistd.h>
extern int syslog_receiver(const char *,const char *);
int main(void)
{
    char dir[]="/tmp/fractal-syslog-XXXXXX",path[108],log[108];
    assert(mkdtemp(dir));
    snprintf(path,sizeof(path),"%s/socket",dir); snprintf(log,sizeof(log),"%s/log",dir);
    for (int pass=0;pass<2;pass++) {
        int ready[2]; assert(!pipe(ready));
        pid_t child=fork(); assert(child>=0);
        if (!child) {
            close(ready[0]); char number[32]; snprintf(number,sizeof(number),"%d",ready[1]);
            setenv("FCR_READY_FD",number,1); _exit(syslog_receiver(path,log));
        }
        close(ready[1]); char byte; assert(read(ready[0],&byte,1)==1); close(ready[0]);
        int fd=socket(AF_UNIX,SOCK_DGRAM,0); assert(fd>=0);
        struct sockaddr_un a={.sun_family=AF_UNIX}; strcpy(a.sun_path,path);
        const char *message=pass?"<30>kata: restarted":"<30>kata: ready";
        assert(sendto(fd,message,strlen(message),0,(struct sockaddr *)&a,sizeof(a))==(ssize_t)strlen(message)); close(fd);
        int found=0;
        for (int i=0;i<100 && !found;i++) {
            FILE *f=fopen(log,"r"); char buffer[256];
            if (f) { while (fgets(buffer,sizeof(buffer),f)) if (strstr(buffer,message)) found=1; fclose(f); }
            if (!found) usleep(10000);
        }
        assert(found); kill(child,SIGTERM); assert(waitpid(child,NULL,0)==child);
    }
    unlink(path); FILE *file=fopen(path,"w"); assert(file); fclose(file);
    assert(syslog_receiver(path,log)==1);
    unlink(path); unlink(log); rmdir(dir);
    puts("PASS syslog delivery, restart with stale socket, and non-socket path protection");
}
