package runtime

import (
	"math"
	"strings"
)

// ---------------------------------------------------------------
// Naive Bayes Text Classifier for Command Intent Inference
// ---------------------------------------------------------------
// This implements a multinomial Naive Bayes classifier with
// Laplace smoothing, trained on a labeled corpus of Linux commands.
// It produces calibrated posterior probabilities for each intent class.
//
// Academic reference:
//   McCallum, A. & Nigam, K. (1998). "A Comparison of Event Models
//   for Naive Bayes Text Classification." AAAI Workshop on Learning
//   for Text Categorization.
// ---------------------------------------------------------------

type IntentClass string

const (
	IntentNetwork    IntentClass = "network-task"
	IntentCompute    IntentClass = "compute-heavy"
	IntentDangerous  IntentClass = "untrusted-dangerous"
	IntentFileIO     IntentClass = "file-io"
	IntentGeneral    IntentClass = "general-task"
)

// ClassPrior holds the prior probability and word likelihoods for a class.
type ClassPrior struct {
	Prior       float64
	WordCounts  map[string]int
	TotalWords  int
}

// NaiveBayesClassifier is a multinomial Naive Bayes with Laplace smoothing.
type NaiveBayesClassifier struct {
	Classes    map[IntentClass]*ClassPrior
	Vocabulary map[string]bool
	VocabSize  int
}

// trainingCorpus is a labeled dataset of Linux commands and their intents.
// Each entry: (command_text, intent_class).
var trainingCorpus = []struct {
	Text   string
	Intent IntentClass
}{
	// --- Network tasks (need network access) ---
	{"curl -s http://example.com", IntentNetwork},
	{"curl -X POST -d data http://api.server.com/endpoint", IntentNetwork},
	{"wget http://releases.ubuntu.com/file.iso", IntentNetwork},
	{"wget --mirror http://website.com", IntentNetwork},
	{"ping -c 4 google.com", IntentNetwork},
	{"ping 8.8.8.8", IntentNetwork},
	{"apt update && apt install nginx", IntentNetwork},
	{"apt-get install python3-pip", IntentNetwork},
	{"pip install requests flask numpy", IntentNetwork},
	{"npm install express", IntentNetwork},
	{"git clone https://github.com/user/repo.git", IntentNetwork},
	{"git pull origin main", IntentNetwork},
	{"ssh user@server.com", IntentNetwork},
	{"scp file.txt user@host:/path", IntentNetwork},
	{"rsync -avz /local/ user@host:/remote/", IntentNetwork},
	{"nslookup google.com", IntentNetwork},
	{"dig example.com", IntentNetwork},
	{"traceroute google.com", IntentNetwork},
	{"netcat -l 8080", IntentNetwork},
	{"nc -zv host 80", IntentNetwork},
	{"docker pull nginx", IntentNetwork},
	{"yum install httpd", IntentNetwork},
	{"pacman -S firefox", IntentNetwork},
	{"brew install node", IntentNetwork},

	// --- Compute-heavy tasks (CPU/memory intensive, should be airgapped) ---
	{"python train_model.py --epochs 100", IntentCompute},
	{"python3 -c 'import numpy; print(numpy.dot(a,b))'", IntentCompute},
	{"python script.py", IntentCompute},
	{"gcc -O2 -o program main.c", IntentCompute},
	{"gcc -Wall -g main.c -lm", IntentCompute},
	{"g++ -std=c++17 -O3 solver.cpp -o solver", IntentCompute},
	{"make -j$(nproc)", IntentCompute},
	{"javac Main.java && java Main", IntentCompute},
	{"go build ./...", IntentCompute},
	{"rustc --edition 2021 main.rs", IntentCompute},
	{"awk 'BEGIN{for(i=0;i<1e8;i++) s+=i; print s}'", IntentCompute},
	{"awk '{sum+=$1} END{print sum}' data.csv", IntentCompute},
	{"sed -n 's/pattern/replacement/gp' largefile.txt", IntentCompute},
	{"sort -t, -k3 -n largefile.csv", IntentCompute},
	{"grep -rn 'pattern' /large/directory/", IntentCompute},
	{"find / -name '*.log' -exec wc -l {} +", IntentCompute},
	{"stress-ng --vm 2 --vm-bytes 512M --timeout 30s", IntentCompute},
	{"stress --cpu 4 --timeout 60", IntentCompute},
	{"ffmpeg -i video.mp4 -c:v libx264 output.mkv", IntentCompute},
	{"tar -czf archive.tar.gz /large/directory", IntentCompute},
	{"openssl speed aes-256-cbc", IntentCompute},
	{"node heavy_computation.js", IntentCompute},
	{"julia simulate.jl", IntentCompute},
	{"matlab -batch 'run_simulation'", IntentCompute},
	{"R -e 'source(\"analysis.R\")'", IntentCompute},

	// --- Dangerous/destructive commands (maximum isolation) ---
	{"rm -rf /", IntentDangerous},
	{"rm -rf /*", IntentDangerous},
	{"rm -rf /home/user/important/", IntentDangerous},
	{"chmod 777 /etc/shadow", IntentDangerous},
	{"chmod -R 777 /", IntentDangerous},
	{"chown root:root /etc/passwd", IntentDangerous},
	{"dd if=/dev/zero of=/dev/sda bs=1M", IntentDangerous},
	{"dd if=/dev/urandom of=/dev/sda", IntentDangerous},
	{"mkfs.ext4 /dev/sda1", IntentDangerous},
	{":(){:|:&};:", IntentDangerous},
	{"echo 'malware' > /etc/crontab", IntentDangerous},
	{"crontab -r", IntentDangerous},
	{"iptables -F && iptables -X", IntentDangerous},
	{"kill -9 1", IntentDangerous},
	{"killall -9 systemd", IntentDangerous},
	{"shutdown -h now", IntentDangerous},
	{"reboot", IntentDangerous},
	{"mount /dev/sda1 /mnt", IntentDangerous},
	{"umount -f /", IntentDangerous},
	{"echo c > /proc/sysrq-trigger", IntentDangerous},
	{"modprobe -r module", IntentDangerous},
	{"insmod rootkit.ko", IntentDangerous},
	{"passwd root", IntentDangerous},
	{"useradd backdoor", IntentDangerous},
	{"visudo", IntentDangerous},

	// --- File I/O tasks (read/write heavy, moderate isolation) ---
	{"cat /var/log/syslog", IntentFileIO},
	{"tail -f /var/log/messages", IntentFileIO},
	{"head -n 100 data.csv", IntentFileIO},
	{"wc -l *.txt", IntentFileIO},
	{"ls -la /home/user/", IntentFileIO},
	{"find . -name '*.py' -type f", IntentFileIO},
	{"cp -r /source /destination", IntentFileIO},
	{"mv file.txt /new/location/", IntentFileIO},
	{"mkdir -p /project/src/components", IntentFileIO},
	{"touch newfile.txt", IntentFileIO},
	{"stat /etc/hostname", IntentFileIO},
	{"df -h", IntentFileIO},
	{"du -sh /var/log", IntentFileIO},
	{"tree /project/", IntentFileIO},
	{"diff file1.txt file2.txt", IntentFileIO},
	{"tee output.log", IntentFileIO},
	{"ln -s /target /link", IntentFileIO},

	// --- General tasks (low risk, standard isolation) ---
	{"echo 'hello world'", IntentGeneral},
	{"date", IntentGeneral},
	{"whoami", IntentGeneral},
	{"hostname", IntentGeneral},
	{"uname -a", IntentGeneral},
	{"uptime", IntentGeneral},
	{"id", IntentGeneral},
	{"env", IntentGeneral},
	{"printenv PATH", IntentGeneral},
	{"pwd", IntentGeneral},
	{"cal", IntentGeneral},
	{"sleep 10", IntentGeneral},
	{"true", IntentGeneral},
	{"false", IntentGeneral},
	{"yes | head -n 5", IntentGeneral},
	{"seq 1 100", IntentGeneral},
	{"expr 2 + 2", IntentGeneral},
	{"bc -l <<< '22/7'", IntentGeneral},
	{"man ls", IntentGeneral},
	{"help", IntentGeneral},
	{"history", IntentGeneral},
	{"alias ll='ls -la'", IntentGeneral},
	{"export VAR=value", IntentGeneral},
	{"set -e", IntentGeneral},
	{"clear", IntentGeneral},
	{"reset", IntentGeneral},
}

// tokenize splits command text into word tokens for classification.
func tokenize(text string) []string {
	text = strings.ToLower(text)
	// Replace common separators with spaces
	for _, sep := range []string{"/", "-", ".", ",", ";", "|", "&", "(", ")", "{", "}", "'", "\"", "=", ">", "<", ":", "+"} {
		text = strings.ReplaceAll(text, sep, " ")
	}
	raw := strings.Fields(text)
	tokens := make([]string, 0, len(raw))
	for _, t := range raw {
		if len(t) >= 2 { // filter very short tokens
			tokens = append(tokens, t)
		}
	}
	return tokens
}

// NewNaiveBayesClassifier trains a new classifier on the built-in corpus.
func NewNaiveBayesClassifier() *NaiveBayesClassifier {
	nbc := &NaiveBayesClassifier{
		Classes:    make(map[IntentClass]*ClassPrior),
		Vocabulary: make(map[string]bool),
	}

	// Count documents per class
	classDocs := make(map[IntentClass]int)
	totalDocs := len(trainingCorpus)
	for _, entry := range trainingCorpus {
		classDocs[entry.Intent]++
	}

	// Initialize class priors
	for class, count := range classDocs {
		nbc.Classes[class] = &ClassPrior{
			Prior:      float64(count) / float64(totalDocs),
			WordCounts: make(map[string]int),
		}
	}

	// Build vocabulary and word counts per class
	for _, entry := range trainingCorpus {
		tokens := tokenize(entry.Text)
		cp := nbc.Classes[entry.Intent]
		for _, token := range tokens {
			cp.WordCounts[token]++
			cp.TotalWords++
			nbc.Vocabulary[token] = true
		}
	}

	nbc.VocabSize = len(nbc.Vocabulary)
	return nbc
}

// ClassificationResult holds the predicted class and confidence scores.
type ClassificationResult struct {
	PredictedIntent IntentClass            `json:"predicted_intent"`
	Confidence      float64                `json:"confidence"`
	Probabilities   map[IntentClass]float64 `json:"probabilities"`
}

// Classify returns the most probable intent class for a command string.
func (nbc *NaiveBayesClassifier) Classify(command string) ClassificationResult {
	tokens := tokenize(command)
	logProbs := make(map[IntentClass]float64)

	for class, cp := range nbc.Classes {
		// Start with log prior
		logProb := math.Log(cp.Prior)

		// Add log likelihoods with Laplace smoothing
		for _, token := range tokens {
			wordCount := cp.WordCounts[token] // 0 if unseen
			// P(word|class) = (count + 1) / (totalWords + vocabSize)
			likelihood := float64(wordCount+1) / float64(cp.TotalWords+nbc.VocabSize)
			logProb += math.Log(likelihood)
		}

		logProbs[class] = logProb
	}

	// Find max log probability (for numerical stability in softmax)
	maxLogProb := math.Inf(-1)
	var bestClass IntentClass
	for class, lp := range logProbs {
		if lp > maxLogProb {
			maxLogProb = lp
			bestClass = class
		}
	}

	// Convert to calibrated probabilities via log-sum-exp
	sumExp := 0.0
	for _, lp := range logProbs {
		sumExp += math.Exp(lp - maxLogProb)
	}

	probabilities := make(map[IntentClass]float64)
	for class, lp := range logProbs {
		probabilities[class] = math.Exp(lp-maxLogProb) / sumExp
	}

	return ClassificationResult{
		PredictedIntent: bestClass,
		Confidence:      probabilities[bestClass],
		Probabilities:   probabilities,
	}
}
