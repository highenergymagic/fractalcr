#include "pager.h"
#include <assert.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
struct fixture { const char *keys; char output[32768]; size_t used; };
static int key(void *ctx){struct fixture *f=ctx;return *f->keys?(unsigned char)*f->keys++:-1;}
static void write_text(void *ctx,const char *text,size_t n){struct fixture *f=ctx;assert(f->used+n<sizeof(f->output));memcpy(f->output+f->used,text,n);f->used+=n;f->output[f->used]=0;}
static void start(struct pager *p,struct fixture *f,const char *keys,unsigned length,unsigned width,int locked)
{
    memset(f,0,sizeof(*f));f->keys=keys;p->context=f;p->read_key=key;p->write=write_text;pager_reset(p,length,width,locked);
}
int main(void)
{
    struct pager p={0};struct fixture f;
    start(&p,&f,"  ",3,80,0);
    pager_write(&p,"one\ntwo\nthree\nfour\nfive\n");pager_finish(&p);
    assert(strstr(f.output," --More-- ") && strstr(f.output,"five\n") && !p.aborted);
    start(&p,&f,"\nq",3,80,0);
    pager_write(&p,"one\ntwo\nthree\nfour\nfive\n");
    assert(strstr(f.output,"three\n") && !strstr(f.output,"four\n") && p.aborted);
    start(&p,&f,"/target\n ",3,80,0);
    pager_write(&p,"one\ntwo\nskip\n");pager_write(&p,"tar");pager_write(&p,"get\nafter\nlast");pager_finish(&p);
    assert(strstr(f.output,"filtering...\n") && !strstr(f.output,"skip\n") && strstr(f.output,"target\nafter\n") && strstr(f.output,"last"));
    start(&p,&f,"+keep\n  ",3,80,0);
    pager_write(&p,"one\ntwo\nkeep-a\ndrop\nkeep-b\nkeep-c\n");
    assert(!strstr(f.output,"drop\n") && strstr(f.output,"keep-a\n") && strstr(f.output,"keep-c\n") && p.locked);
    start(&p,&f,"-omit\n  ",3,80,0);
    pager_write(&p,"one\ntwo\nomit\nkeep\nomit-again\nlast\n");
    assert(strstr(f.output,"keep\nlast\n") && !strstr(f.output,"omit-again\n"));
    start(&p,&f,"+keep\n/last\n",3,80,0);
    pager_write(&p,"one\ntwo\nkeep-a\nkeep-b\nlast-drop\nkeep-skipped\nkeep-last\n");
    assert(strstr(f.output,"keep-last\n") && !strstr(f.output,"last-drop\n") && !strstr(f.output,"keep-skipped\n") && p.filter_active);
    start(&p,&f,"/missing\n",3,80,0);
    pager_write(&p,"one\ntwo\nthree\nfour\n");pager_finish(&p);
    assert(!strstr(f.output,"three\n") && !p.aborted);
    start(&p,&f,"/[\n ",3,80,0);
    pager_write(&p,"one\ntwo\nthree\n");
    assert(strstr(f.output,"Invalid regular expression") && strstr(f.output,"three\n"));
    start(&p,&f,"+ /last\n",3,80,1);
    pager_write(&p,"one\ntwo\nthree\nfour\nlast\n");
    assert(!p.filter_active && strstr(f.output,"last\n"));
    for(int stop=0;stop<4;stop++){
        const char *keys[]={"","\003","\032","\036"};
        start(&p,&f,keys[stop],2,80,0);pager_write(&p,"one\ntwo\n");
        assert(p.aborted && !strstr(f.output,"two\n"));
    }
    start(&p,&f,"",0,80,0);pager_write(&p,"unfiltered\n");assert(!strcmp(f.output,"unfiltered\n"));
    start(&p,&f,"",3,4,0);pager_write(&p,"1234\nnext\n");assert(!strstr(f.output,"More") && !p.aborted);
    start(&p,&f," ",2,4,0);pager_write(&p,"12345\n");assert(strstr(f.output,"More") && !p.aborted);
    start(&p,&f,"",0,80,0);pager_write(&p,"fragment");pager_write(&p,"ed");pager_finish(&p);assert(!strcmp(f.output,"fragmented"));
    start(&p,&f,"",0,80,0);
    char *large=malloc(7U*1024U*1024U+2);assert(large);
    memset(large,'x',7U*1024U*1024U+1);large[7U*1024U*1024U+1]=0;
    pager_write(&p,large);free(large);assert(p.aborted && strstr(f.output,"Output line exceeds display limit"));
    pager_destroy(&p);puts("pager tests passed");return 0;
}
