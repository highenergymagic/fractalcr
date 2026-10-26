/* Runs only on disposable regular-file fixtures supplied by the test runner. */
#define main storage_service_main
#include "fractal-storage.c"
#undef main
#include <assert.h>
int main(int argc,char **argv)
{
 assert(argc==2);
 struct request bad={.magic=MAGIC,.version=1,.operation=2,.profile=1,.count=1};
 strcpy(bad.disks[0],"sda");assert(validate(&bad)==1);
 bad.profile=0;strcpy(bad.disks[0],"../../dev/sda");assert(validate(&bad)==1);
 bad.operation=3;assert(validate(&bad)==1);
 memset(bad.disks[0],'a',64);assert(validate(&bad)==1);
 assert(!deny_exec());
 char *forbidden[]={"/bin/true",NULL};
 assert(execv(forbidden[0],forbidden)==-1 && errno==EPERM);
 pid_t child=fork();assert(child>=0);
 if(!child){char *args[]={"fractal-storage","-L","FCR_TEST","-d","single","-m","single",argv[1],NULL};_exit(mkfs_main(8,args));}
 int result;assert(waitpid(child,&result,0)==child && WIFEXITED(result) && WEXITSTATUS(result)==0);
 int fd=open(argv[1],O_RDONLY);assert(fd>=0);
 unsigned char before[4096],after[4096];assert(pread(fd,before,sizeof(before),65536)==sizeof(before));
 assert(!memcmp(before+64,"_BHRfS_M",8));
 child=fork();assert(child>=0);
 if(!child){char *args[]={"fractal-storage","-L","REPLACE",argv[1],NULL};_exit(mkfs_main(4,args));}
 assert(waitpid(child,&result,0)==child && WIFEXITED(result) && WEXITSTATUS(result)!=0);
 assert(pread(fd,after,sizeof(after),65536)==sizeof(after) && !memcmp(before,after,sizeof(before)));
 close(fd);puts("STORAGE_API_GUARD_FORMAT_AND_OVERWRITE_PROTECTION_OK");return 0;
}
