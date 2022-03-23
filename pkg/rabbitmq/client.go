package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dominikus1993/easynetq-hosepipe/pkg/data"
	amqp "github.com/rabbitmq/amqp091-go"
	log "github.com/sirupsen/logrus"
)

type RabbitMqClient interface {
	CreateChannel() (*amqp.Channel, error)
	Close()
}

type RabbitMqPublisher interface {
	Publish(ctx context.Context, exchangeName string, topic string, msg amqp.Publishing) error
	CloseChannel()
}

type RabbitMqSubscriber interface {
	Subscribe(ctx context.Context, exchangeName, queue, topic string) chan data.HosepipeMessage
	CloseChannel()
}

type rabbitMqClient struct {
	connection *amqp.Connection
}

func NewRabbitMqClient(connStr string) (*rabbitMqClient, error) {
	conn, err := amqp.Dial(connStr)
	if err != nil {
		return nil, err
	}
	return &rabbitMqClient{connection: conn}, nil
}

func (client *rabbitMqClient) CreateChannel() (*amqp.Channel, error) {
	return client.connection.Channel()
}

func (client *rabbitMqClient) Close() {
	err := client.connection.Close()
	if err != nil {
		log.WithError(err).Fatalln("error when closing connection")
	}
}

func DeclareExchange(ctx context.Context, channel *amqp.Channel, exchangeName string) error {
	return channel.ExchangeDeclare(
		exchangeName, // name
		"topic",      // type
		true,         // durable
		false,        // auto-deleted
		false,        // internal
		false,        // no-wait
		nil,          // arguments
	)
}

type rabbitMqPublisher struct {
	channel *amqp.Channel
}

func NewRabbitMqPublisher(client RabbitMqClient) (*rabbitMqPublisher, error) {
	channel, err := client.CreateChannel()
	if err != nil {
		return nil, fmt.Errorf("error when creating channel %w", err)
	}
	return &rabbitMqPublisher{channel: channel}, nil
}

func (client *rabbitMqPublisher) Publish(ctx context.Context, exchangeName string, topic string, msg amqp.Publishing) error {
	err := DeclareExchange(ctx, client.channel, exchangeName)
	if err != nil {
		return fmt.Errorf("error when declare exchange %w", err)
	}

	return client.channel.Publish(exchangeName, topic, false, false, msg)
}

type rabbitMqSubscriber struct {
	channel *amqp.Channel
}

func NewRabbitMqSubscriber(client RabbitMqClient) (*rabbitMqSubscriber, error) {
	channel, err := client.CreateChannel()
	if err != nil {
		return nil, fmt.Errorf("error when creating channel %w", err)
	}
	return &rabbitMqSubscriber{channel: channel}, nil
}

func (client *rabbitMqSubscriber) Subscribe(ctx context.Context, exchangeName, queue, topic string) <-chan *data.HosepipeMessage {
	res := make(chan *data.HosepipeMessage)

	q, err := client.channel.QueueDeclare(
		queue, // name
		true,  // durable
		false, // delete when usused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)

	if err != nil {
		log.WithError(err).Fatalln("error when declaring queue")
	}

	msgs, err := client.channel.Consume(
		q.Name,     // queue
		"hosepipe", // consumer
		true,       // auto-ack
		false,      // exclusive
		false,      // no-local
		false,      // no-wait
		nil,        // args
	)

	if err != nil {
		log.WithError(err).Fatalln("error when consuming queue")
	}

	go func(stream chan<- *data.HosepipeMessage) {
		for msg := range msgs {
			var res data.HosepipeMessage
			err := json.Unmarshal(msg.Body, &res)
			if err != nil {
				log.WithError(err).Errorln("Error in subscribe method")
			} else {
				stream <- &res
			}
		}
		close(stream)
	}(res)
	return res
}