package main

/*

//#include <glib.h>
#include <stdio.h>
#include <stdlib.h>
//#include <string.h>
#include <libnotify/notify.h>

//// The gateway function
//int callOnMeGo_cgo(NotifyNotification *notification, char *action, gpointer user_data)
//{
//    printf("C.callOnMeGo_cgo(): called with arg = %s\n", action);
//    return callOnMeGo(notification, action, user_data);
//}
//
//void c_callback(NotifyNotification *not, char *action, gpointer user_data)
//{
//	printf("in c_callback\n");
//	callOnMeGo(not, action, user_data);
//}
//
//void bridge(NotifyNotification *notification)
//{
//	printf("in bridge\n");
//
//	NotifyNotification *bridged_notif = notification;
//
//	char *action = (char *) malloc(10 * sizeof(char));
//	char *label = (char *) malloc(10 * sizeof(char));
//
//	strcpy(action, "action");
//	strcpy(label, "label");
//
//	notify_notification_add_action(bridged_notif, action, label, &c_callback, 0, 0);
//
////	C.notify_notification_add_action((*C.struct__NotifyNotification)(unsafe.Pointer(hello)), C.CString("action"), C.CString("label"), (C.NotifyActionCallback)(unsafe.Pointer(C.callOnMeGo_cgo)), nil, nil)
//
//}

extern void callOnMeGo(NotifyNotification *notification, char *action, gpointer user_data);

void c_callback(NotifyNotification *notification, char *action, gpointer user_data)
{
	printf("IN CALLBACK WHAT\n");// with action %s\n", action);
}

void sendNotification(char *action, char *label, char *summary, char *body)
{
	notify_init ("Hello world!");
	NotifyNotification * Hello = notify_notification_new ("Hello world", "This is an example notification.", "dialog-information");
	notify_notification_add_action(Hello, action, label, NOTIFY_ACTION_CALLBACK(&callOnMeGo), NULL, NULL);
	//notify_notification_add_action(Hello, action, label, NOTIFY_ACTION_CALLBACK(&c_callback), NULL, NULL);
	GError *gerror = NULL;
	printf("err: %d\n", (int) notify_notification_show (Hello, &gerror));
	printf("gerror: %p\n", gerror);
	if (gerror != NULL) {
		printf("gquark: %d, code: %d, message: '%s'\n", (int) gerror->domain,
			(int) gerror->code, gerror->message);
	}
	g_object_unref(G_OBJECT(Hello));

	//notify_uninit();

//	NotifyNotification *notification = notify_notification_new(
//		summary,
//		body,
//		0
//	);
//
//	gboolean err = notify_notification_show(notification, 0);
	//printf("err: %d\n", err);
}
*/
import "C"
