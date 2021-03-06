package kafkasecret

import (
	"context"
	"github.com/kyma-incubator/knative-kafka/components/controller/constants"
	injectionclient "github.com/kyma-incubator/knative-kafka/components/controller/pkg/client/injection/client"
	"github.com/kyma-incubator/knative-kafka/components/controller/pkg/client/injection/informers/knativekafka/v1alpha1/kafkachannel"
	kafkasecretreconciler "github.com/kyma-incubator/knative-kafka/components/controller/pkg/client/injection/reconciler/knativekafka/v1alpha1/kafkasecret"
	"github.com/kyma-incubator/knative-kafka/components/controller/pkg/env"
	"github.com/kyma-incubator/knative-kafka/components/controller/pkg/kafkasecretinformer"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"knative.dev/eventing/pkg/logging"
	"knative.dev/eventing/pkg/reconciler"
	"knative.dev/pkg/client/injection/kube/informers/apps/v1/deployment"
	"knative.dev/pkg/client/injection/kube/informers/core/v1/service"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"os"
)

// Create A New KafkaSecret Controller
func NewController(ctx context.Context, cmw configmap.Watcher) *controller.Impl {

	// Get A Logger
	logger := logging.FromContext(ctx)

	// Get The Needed Informers
	kafkaSecretInformer := kafkasecretinformer.Get(ctx)
	kafkachannelInformer := kafkachannel.Get(ctx)
	deploymentInformer := deployment.Get(ctx)
	serviceInformer := service.Get(ctx)

	// Load The Environment Variables
	environment, err := env.GetEnvironment(logger)
	if err != nil {
		logger.Panic("Failed To Load Environment Variables - Terminating!", zap.Error(err))
		os.Exit(1)
	}

	// Create The KafkaSecret Reconciler
	r := &Reconciler{
		Base:               reconciler.NewBase(ctx, constants.KafkaSecretControllerAgentName, cmw),
		environment:        environment,
		kafkaChannelClient: injectionclient.Get(ctx),
		kafkachannelLister: kafkachannelInformer.Lister(),
		deploymentLister:   deploymentInformer.Lister(),
		serviceLister:      serviceInformer.Lister(),
	}

	// Create A New KafkaSecret Controller Impl With The Reconciler
	controllerImpl := kafkasecretreconciler.NewImpl(ctx, r)

	// Configure The Informers' EventHandlers
	r.Logger.Info("Setting Up EventHandlers")
	kafkaSecretInformer.Informer().AddEventHandler(
		controller.HandleAll(controllerImpl.Enqueue),
	)
	serviceInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(corev1.SchemeGroupVersion.WithKind(constants.SecretKind)),
		Handler:    controller.HandleAll(controllerImpl.EnqueueControllerOf),
	})
	deploymentInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.Filter(corev1.SchemeGroupVersion.WithKind(constants.SecretKind)),
		Handler:    controller.HandleAll(controllerImpl.EnqueueControllerOf),
	})
	kafkachannelInformer.Informer().AddEventHandler(
		controller.HandleAll(enqueueSecretOfKafkaChannel(controllerImpl)),
	)

	// Return The KafkaSecret Controller Impl
	return controllerImpl
}

// Graceful Shutdown Hook
func Shutdown() {
	// Nothing To Cleanup
}

// Enqueue The Kafka Secret Associated With The Specified KafkaChannel
func enqueueSecretOfKafkaChannel(controller *controller.Impl) func(obj interface{}) {
	return func(obj interface{}) {
		if object, ok := obj.(metav1.Object); ok {
			labels := object.GetLabels()
			if len(labels) > 0 {
				secretName := labels[constants.KafkaSecretLabel]
				if len(secretName) > 0 {
					controller.EnqueueKey(types.NamespacedName{
						Namespace: constants.KnativeEventingNamespace,
						Name:      secretName,
					})
				}
			}
		}
	}
}
