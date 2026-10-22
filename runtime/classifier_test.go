package runtime

import (
	"testing"
)

func TestClassifierAccuracy(t *testing.T) {
	classifier := NewNaiveBayesClassifier()

	testCases := []struct {
		command  string
		expected IntentClass
	}{
		// Network tasks
		{"curl -fsSL https://get.docker.com", IntentNetwork},
		{"wget http://example.com/archive.tar.gz", IntentNetwork},
		{"git clone https://github.com/torvalds/linux", IntentNetwork},
		{"ssh -i key.pem user@remote-server", IntentNetwork},
		{"nc -zv 192.168.1.1 80", IntentNetwork},
		{"ping -c 4 8.8.8.8", IntentNetwork},

		// Compute / Batch tasks
		{"gcc -O3 -march=native main.c -o app", IntentCompute},
		{"make -j$(nproc)", IntentCompute},
		{"ffmpeg -i input.mp4 -c:v libx264 out.mp4", IntentCompute},
		{"python train_model.py --epochs 100", IntentCompute},
		{"stress --cpu 4 --timeout 60", IntentCompute},

		// Dangerous commands
		{"rm -rf / --no-preserve-root", IntentDangerous},
		{"mkfs.ext4 /dev/sda1", IntentDangerous},
		{"dd if=/dev/zero of=/dev/sda bs=1M", IntentDangerous},
		{"chmod 777 /etc/shadow", IntentDangerous},

		// File I/O tasks
		{"cat /var/log/syslog", IntentFileIO},
		{"tail -f /var/log/messages", IntentFileIO},
		{"ls -la /home/user/", IntentFileIO},
		{"find . -name '*.py' -type f", IntentFileIO},
		{"cp -r /source /destination", IntentFileIO},

		// General tasks
		{"echo 'hello world'", IntentGeneral},
		{"date", IntentGeneral},
		{"whoami", IntentGeneral},
		{"hostname", IntentGeneral},
		{"pwd", IntentGeneral},
	}

	correct := 0
	for _, tc := range testCases {
		result := classifier.Classify(tc.command)
		if result.PredictedIntent == tc.expected {
			correct++
		} else {
			t.Logf("MISMATCH for '%s': got %s (conf=%.2f), expected %s",
				tc.command, result.PredictedIntent, result.Confidence, tc.expected)
		}
	}

	accuracy := float64(correct) / float64(len(testCases)) * 100
	t.Logf("Classification accuracy: %.1f%% (%d/%d)", accuracy, correct, len(testCases))
	if accuracy < 85.0 {
		t.Fatalf("Accuracy too low: %.1f%% (required >= 85%%)", accuracy)
	}
}

func BenchmarkClassifierInference(b *testing.B) {
	classifier := NewNaiveBayesClassifier()
	cmds := []string{
		"curl -s https://api.github.com/repos",
		"gcc -O2 -Wall main.c -lm",
		"rm -rf /tmp/cache/*",
		"cat /var/log/syslog",
		"whoami",
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cmd := cmds[i%len(cmds)]
		_ = classifier.Classify(cmd)
	}
}

func BenchmarkPolicyCompilation(b *testing.B) {
	intents := []string{
		"network-task",
		"compute-heavy",
		"untrusted-dangerous",
		"file-io",
		"general-task",
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		intent := intents[i%len(intents)]
		_ = CompileIntent(intent)
	}
}
