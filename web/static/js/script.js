function updateDurations() {
  const entries = document.querySelectorAll('[data-start-time]');
  const now = Math.floor(Date.now() / 1000);

  entries.forEach(entry => {
      const startTime = parseInt(entry.dataset.startTime);
      const durationSeconds = now - startTime;

      const hours = Math.floor(durationSeconds / 3600);
      const minutes = Math.floor((durationSeconds % 3600) / 60);
      const seconds = durationSeconds % 60;

      let durationStr = '';
      if (hours > 0) durationStr += `${hours}h `;
      if (minutes > 0) durationStr += `${minutes}m `;
      durationStr += `${seconds}s`;

      const startTimeStr = new Date(startTime * 1000).toLocaleTimeString();
      entry.title = `Started: ${startTimeStr} (Duration: ${durationStr})`;
  });
}

// Update durations immediately and every second
updateDurations();
setInterval(updateDurations, 1000);
