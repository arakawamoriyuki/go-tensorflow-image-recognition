package tf_image_recognition

import (
	"bufio"
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/julienschmidt/httprouter"
	tf "github.com/tensorflow/tensorflow/tensorflow/go"
)

type ClassifyResult struct {
	Filename string        `json:"filename"`
	Labels   []LabelResult `json:"labels"`
}

type LabelResult struct {
	Label       string  `json:"label"`
	Probability float32 `json:"probability"`
}

var (
	graphModel   *tf.Graph
	sessionModel *tf.Session
	labels       []string
)

func Main() {
	if err := LoadModel(); err != nil {
		log.Fatal(err)
		return
	}

	r := httprouter.New()
	r.POST("/recognize", RecognizeHandler)
	log.Fatal(http.ListenAndServe(":8080", r))
}

func LoadModel() error {
	// Load inception model
	model, err := ioutil.ReadFile("/model/tensorflow_inception_graph.pb")
	if err != nil {
		return err
	}
	graphModel = tf.NewGraph()
	if err := graphModel.Import(model, ""); err != nil {
		return err
	}

	sessionModel, err = tf.NewSession(graphModel, nil)
	if err != nil {
		log.Fatal(err)
	}

	// Load labels
	labelsFile, err := os.Open("/model/imagenet_comp_graph_label_strings.txt")
	if err != nil {
		return err
	}
	defer labelsFile.Close()
	scanner := bufio.NewScanner(labelsFile)
	// Labels are separated by newlines
	for scanner.Scan() {
		labels = append(labels, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func RecognizeHandler(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	// Read image
	imageFile, header, err := r.FormFile("image")
	// Will contain filename and extension
	imageName := strings.Split(header.Filename, ".")
	if err != nil {
		ResponseError(w, "Could not read image", http.StatusBadRequest)
		return
	}
	defer imageFile.Close()
	var imageBuffer bytes.Buffer
	// Copy image data to a buffer
	io.Copy(&imageBuffer, imageFile)

	// ...
	// Make tensor
	tensor, err := MakeTensorFromImage(&imageBuffer, imageName[:1][0])
	if err != nil {
		ResponseError(w, "Invalid image", http.StatusBadRequest)
		return
	}

	// Run inference
	output, err := sessionModel.Run(
		map[tf.Output]*tf.Tensor{
			graphModel.Operation("input").Output(0): tensor,
		},
		[]tf.Output{
			graphModel.Operation("output").Output(0),
		},
		nil)
	if err != nil {
		ResponseError(w, "Could not run inference", http.StatusInternalServerError)
		return
	}

	// Return best labels
	ResponseJSON(w, ClassifyResult{
		Filename: header.Filename,
		Labels:   FindBestLabels(output[0].Value().([][]float32)[0]),
	})
}

type ByProbability []LabelResult

func (a ByProbability) Len() int           { return len(a) }
func (a ByProbability) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByProbability) Less(i, j int) bool { return a[i].Probability > a[j].Probability }

func FindBestLabels(probabilities []float32) []LabelResult {
	// Make a list of label/probability pairs
	var resultLabels []LabelResult
	for i, p := range probabilities {
		if i >= len(labels) {
			break
		}
		resultLabels = append(resultLabels, LabelResult{Label: labels[i], Probability: p})
	}
	// Sort by probability
	sort.Sort(ByProbability(resultLabels))
	// Return top 5 labels
	return resultLabels[:5]
}
