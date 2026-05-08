const backendURL = window.location.port === "4173" ? `${window.location.protocol}//${window.location.hostname}/api` : "/api";

document.addEventListener("DOMContentLoaded", () => {
  const searchButton = document.getElementById("search-button");
  const cringeButton = document.getElementById("cringe-button");
  const searchBar = document.getElementById("search-bar");
  let infoContainer;
  let formattedInfo;
  let html;

  if (searchButton) {
    searchButton.addEventListener("click", () => {
      const query = searchBar.value.trim();
      if (query) {
        search(query);
      } else {
        alert("Please enter a search query");
      }
    });
  }

  if (cringeButton) {
    cringeButton.addEventListener("click", () => {
      cringe();
    });
  }

  if (searchBar) {
    searchBar.value = "";
    searchBar.placeholder = "Search for something!";
    searchBar.addEventListener("keydown", (event) => {
      if (event.key == "Enter") {
        const query = searchBar.value.trim();
        if (query) {
          search(query);
        } else {
          alert("Please enter a search query");
        }
      }
    });
  }

  console.log(`Pinging backend at ${backendURL}...`);
  // Check if backend is ready before enabling search.
  fetch(`${backendURL}/health/ready`)
    .then((res) => res.json())
    .then((data) => {
      if (data.status === "up" || data.status === "ready") {
        infoContainer = document.getElementById("info-container");

        formattedInfo = data.pages ? getFormattedInfoString(data.pages) : "Search is online";

        html = `<span class="info">${formattedInfo}</span>`;
        infoContainer.innerHTML = html;
        renderPageCount();
      } else if (data.status === "down") {
        serverDown();
      }
    })
    .catch((error) => {
      console.error("Error fetching data:", error);
      serverDown();
    });

  function serverDown() {
    infoContainer = document.getElementById("info-container");
    infoContainer.innerHTML = `Server down - please come back later`;
    searchBar.disabled = true;
    searchBar.placeholder = "hello";
    searchBar.value = "Please come back later...";
    searchButton.disabled = true;
    cringeButton.disabled = true;
  }
});

function getFormattedInfoString(pageCount) {
  const formatter = new Intl.NumberFormat("en", { notation: "compact" });

  let formattedString = "Contains ";
  const approxCount = formatter.format(pageCount);

  let order = approxCount.slice(-1);
  switch (order) {
    case "K":
      formattedString += `~ ${approxCount.slice(0, -1)} thousand`;
      break;
    case "M":
      formattedString += `~ ${approxCount.slice(0, -1)} million`;
      break;
    case "B":
      formattedString += `~ ${approxCount.slice(0, -1)} billion`;
      break;
    default:
      formattedString += `${approxCount}`;
      break;
  }

  formattedString += " results (soon to be much bigger)";

  return formattedString;
}

async function search(query) {
  try {
    const encodedQuery = encodeURIComponent(query);
    const requestUrl = `${backendURL}/search?q=${encodedQuery}`;
    console.log(requestUrl);

    window.location.href = requestUrl;
  } catch (error) {
    console.log(error.message);
  }
}

async function cringe() {
  try {
    const cringeUrl = `${backendURL}/cringe`;
    // const cringeUrl = `http://localhost:8000/api/cringe`;
    window.location.href = cringeUrl;
  } catch (error) {
    // console.log(error.message);
  }
}

function renderPageCount() {
  const pageCountContainer = document.getElementById("page-count-container");
  if (!pageCountContainer) return;

  fetch(`${backendURL}/page-count`)
    .then((res) => res.json())
    .then((data) => {
      if (data.status === "up" && Number.isFinite(data.pages)) {
        const count = new Intl.NumberFormat("en").format(data.pages);
        pageCountContainer.innerHTML = `<span class="page-count">Pages crawled: <strong>${count}</strong></span>`;
      }
    })
    .catch((error) => console.error("Error fetching page count:", error));
}
